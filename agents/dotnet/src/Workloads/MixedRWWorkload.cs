using BenchmarkAgent.Metrics;
using BenchmarkAgent.Protocol;
using CockroachDB.DotNet.Abstractions.Template;
using Microsoft.Extensions.DependencyInjection;
using Npgsql;

namespace BenchmarkAgent.Workloads;

public sealed class MixedRWWorkload(MetricsRecorder recorder, bool library, IServiceProvider? sp = null) : IWorkloadRunner
{
    private CancellationTokenSource? _cts;
    private Task? _runTask;

    public bool IsRunning => _cts is { IsCancellationRequested: false };

    public Task StartAsync(WorkloadConfig config, CancellationToken ct)
    {
        _cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        _runTask = RunAsync(config, _cts.Token);
        return Task.CompletedTask;
    }

    public async Task StopAsync()
    {
        if (_cts is not null) await _cts.CancelAsync();
        if (_runTask is not null) await _runTask.ConfigureAwait(ConfigureAwaitOptions.SuppressThrowing);
    }

    private async Task RunAsync(WorkloadConfig config, CancellationToken ct)
    {
        NpgsqlDataSource ds;
        ICockroachDBTemplate? template = null;

        if (library && sp is not null)
        {
            ds = sp.GetRequiredService<NpgsqlDataSource>();
            template = sp.GetRequiredService<ICockroachDBTemplate>();
        }
        else
        {
            var dsb = new NpgsqlDataSourceBuilder(config.DbUrl);
            dsb.ConnectionStringBuilder.MaxPoolSize = config.Concurrency + 5;
            ds = dsb.Build();
        }

        await EnsureSchemaAsync(ds, ct);
        await SeedAccountsAsync(ds, ct);

        var workers = Enumerable.Range(0, config.Concurrency)
            .Select(_ => WorkerAsync(ds, template, ct))
            .ToArray();

        var deadline = config.DurationSecs > 0
            ? Task.Delay(TimeSpan.FromSeconds(config.DurationSecs), ct)
            : Task.Delay(Timeout.InfiniteTimeSpan, ct);

        await Task.WhenAny(deadline, Task.WhenAll(workers)).ConfigureAwait(false);
        if (!ct.IsCancellationRequested && _cts is not null)
            await _cts.CancelAsync();

        if (!library || sp is null)
            await ds.DisposeAsync();
    }

    private async Task WorkerAsync(NpgsqlDataSource ds, ICockroachDBTemplate? template, CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            var isRead = Random.Shared.NextDouble() < 0.8;
            var sw = System.Diagnostics.Stopwatch.StartNew();
            try
            {
                if (template is not null)
                {
                    if (isRead)
                    {
                        await template.ExecuteFollowerReadAsync(async () =>
                        {
                            await DoReadAsync(ds, ct);
                        });
                    }
                    else
                    {
                        await template.ExecuteAsync(async () =>
                        {
                            await DoWriteAsync(ds, ct);
                        });
                    }
                }
                else
                {
                    if (isRead)
                        await DoReadAsync(ds, ct);
                    else
                        await DoWriteAsync(ds, ct);
                }

                recorder.RecordOp(sw.ElapsedTicks / (TimeSpan.TicksPerMillisecond / 1000));
            }
            catch (OperationCanceledException) { return; }
            catch { recorder.RecordError(); }
        }
    }

    private static async Task DoReadAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        var accountId = Random.Shared.NextInt64(100_000) + 1;
        await using var cmd = ds.CreateCommand("""
            SELECT a.name, a.balance, o.amount, o.status
            FROM bench_accounts a
            LEFT JOIN bench_orders o ON o.account_id = a.id
            WHERE a.id = $1
            LIMIT 20
            """);
        cmd.Parameters.AddWithValue(accountId);
        await using var reader = await cmd.ExecuteReaderAsync(ct);
        while (await reader.ReadAsync(ct)) { }
    }

    private static async Task DoWriteAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var conn = await ds.OpenConnectionAsync(ct);
        await using var tx = await conn.BeginTransactionAsync(ct);

        var accountId = Random.Shared.NextInt64(100_000) + 1;
        var amount = (decimal)(Random.Shared.NextDouble() * 500);

        await using var ins = new NpgsqlCommand(
            "INSERT INTO bench_orders (id, account_id, amount, status) VALUES ($1, $2, $3, 'pending')",
            conn, tx);
        ins.Parameters.AddWithValue(Random.Shared.NextInt64(100_000_000));
        ins.Parameters.AddWithValue(accountId);
        ins.Parameters.AddWithValue(amount);
        await ins.ExecuteNonQueryAsync(ct);

        await using var upd = new NpgsqlCommand(
            "UPDATE bench_accounts SET balance = balance - $1 WHERE id = $2", conn, tx);
        upd.Parameters.AddWithValue(amount);
        upd.Parameters.AddWithValue(accountId);
        await upd.ExecuteNonQueryAsync(ct);

        await tx.CommitAsync(ct);
    }

    private static async Task EnsureSchemaAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd1 = ds.CreateCommand("""
            CREATE TABLE IF NOT EXISTS bench_accounts (
                id INT8 PRIMARY KEY,
                name STRING NOT NULL,
                balance DECIMAL(15,2) NOT NULL DEFAULT 0,
                created_at TIMESTAMPTZ NOT NULL DEFAULT now()
            )
            """);
        await cmd1.ExecuteNonQueryAsync(ct);

        await using var cmd2 = ds.CreateCommand("""
            CREATE TABLE IF NOT EXISTS bench_orders (
                id INT8 PRIMARY KEY,
                account_id INT8 NOT NULL REFERENCES bench_accounts(id),
                amount DECIMAL(15,2) NOT NULL,
                status STRING NOT NULL DEFAULT 'pending',
                created_at TIMESTAMPTZ NOT NULL DEFAULT now()
            )
            """);
        await cmd2.ExecuteNonQueryAsync(ct);
    }

    private static async Task SeedAccountsAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        for (var i = 1; i <= 1000; i++)
        {
            await using var cmd = ds.CreateCommand(
                "UPSERT INTO bench_accounts (id, name, balance) VALUES ($1, $2, $3)");
            cmd.Parameters.AddWithValue((long)i);
            cmd.Parameters.AddWithValue($"account-{i}");
            cmd.Parameters.AddWithValue(10000m);
            await cmd.ExecuteNonQueryAsync(ct);
        }
    }
}
