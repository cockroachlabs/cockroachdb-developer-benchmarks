using BenchmarkAgent.Metrics;
using BenchmarkAgent.Protocol;
using CockroachDB.DotNet.Abstractions.Template;
using Microsoft.Extensions.DependencyInjection;
using Npgsql;

namespace BenchmarkAgent.Workloads;

public sealed class RetryStormWorkload(MetricsRecorder recorder, bool library, IServiceProvider? sp = null) : IWorkloadRunner
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
            dsb.ConnectionStringBuilder.MaxPoolSize = config.Concurrency * 2;
            ds = dsb.Build();
        }

        await EnsureSchemaAsync(ds, ct);
        var hotRows = config.HotRows > 0 ? config.HotRows : 10;
        await SeedHotRowsAsync(ds, hotRows, ct);

        var concurrency = config.Concurrency > 0 ? config.Concurrency : 32;
        var workers = Enumerable.Range(0, concurrency)
            .Select(_ => WorkerAsync(ds, template, hotRows, ct))
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

    private async Task WorkerAsync(NpgsqlDataSource ds, ICockroachDBTemplate? template,
        int hotRows, CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            var rowId = Random.Shared.NextInt64(hotRows) + 1;
            var sw = System.Diagnostics.Stopwatch.StartNew();
            try
            {
                if (template is not null)
                    await LibraryRetryTxnAsync(ds, template, rowId, ct);
                else
                    await VanillaRetryTxnAsync(ds, rowId, ct);

                recorder.RecordOp(sw.ElapsedTicks / (TimeSpan.TicksPerMillisecond / 1000));
            }
            catch (OperationCanceledException) { return; }
            catch { recorder.RecordError(); }
        }
    }

    private static async Task LibraryRetryTxnAsync(NpgsqlDataSource ds, ICockroachDBTemplate template,
        long rowId, CancellationToken ct)
    {
        await template.ExecuteAsync(async () =>
        {
            await ExecTxnAsync(ds, rowId, ct);
        });
    }

    private async Task VanillaRetryTxnAsync(NpgsqlDataSource ds, long rowId, CancellationToken ct)
    {
        const int maxRetries = 5;
        for (var attempt = 0; attempt < maxRetries; attempt++)
        {
            try
            {
                await ExecTxnAsync(ds, rowId, ct);
                if (attempt > 0) recorder.RecordRetrySuccess();
                return;
            }
            catch (PostgresException ex) when (ex.SqlState == "40001")
            {
                recorder.RecordRetry();
                await Task.Delay((attempt + 1) * 10, ct);
            }
        }
        throw new InvalidOperationException($"Max retries exceeded for row {rowId}");
    }

    private static async Task ExecTxnAsync(NpgsqlDataSource ds, long rowId, CancellationToken ct)
    {
        await using var conn = await ds.OpenConnectionAsync(ct);
        await using var tx = await conn.BeginTransactionAsync(ct);
        await using var read = new NpgsqlCommand("SELECT balance FROM bench_accounts WHERE id = $1", conn, tx);
        read.Parameters.AddWithValue(rowId);
        var balance = (decimal)(await read.ExecuteScalarAsync(ct) ?? 0m);

        var delta = (decimal)(Random.Shared.NextDouble() * 200 - 100);
        await using var write = new NpgsqlCommand(
            "UPDATE bench_accounts SET balance = $1 WHERE id = $2", conn, tx);
        write.Parameters.AddWithValue(balance + delta);
        write.Parameters.AddWithValue(rowId);
        await write.ExecuteNonQueryAsync(ct);
        await tx.CommitAsync(ct);
    }

    private static async Task EnsureSchemaAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand("""
            CREATE TABLE IF NOT EXISTS bench_accounts (
                id INT8 PRIMARY KEY,
                name STRING NOT NULL,
                balance DECIMAL(15,2) NOT NULL DEFAULT 0,
                created_at TIMESTAMPTZ NOT NULL DEFAULT now()
            )
            """);
        await cmd.ExecuteNonQueryAsync(ct);
    }

    private static async Task SeedHotRowsAsync(NpgsqlDataSource ds, int count, CancellationToken ct)
    {
        for (var i = 1; i <= count; i++)
        {
            await using var cmd = ds.CreateCommand(
                "UPSERT INTO bench_accounts (id, name, balance) VALUES ($1, $2, $3)");
            cmd.Parameters.AddWithValue((long)i);
            cmd.Parameters.AddWithValue($"hot-account-{i}");
            cmd.Parameters.AddWithValue(1000m);
            await cmd.ExecuteNonQueryAsync(ct);
        }
    }
}
