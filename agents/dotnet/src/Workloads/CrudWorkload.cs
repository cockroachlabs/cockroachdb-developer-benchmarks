using BenchmarkAgent.Metrics;
using BenchmarkAgent.Protocol;
using CockroachDB.DotNet.Abstractions.Template;
using Microsoft.Extensions.DependencyInjection;
using Npgsql;

namespace BenchmarkAgent.Workloads;

public sealed class CrudWorkload(MetricsRecorder recorder, bool library, IServiceProvider? sp = null) : IWorkloadRunner
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

        using var semaphore = new SemaphoreSlim(config.Concurrency, config.Concurrency);
        using var timer = new PeriodicTimer(TimeSpan.FromMilliseconds(500));
        var connTracker = TrackConnectionsAsync(ds, timer, ct);

        var deadline = config.DurationSecs > 0
            ? Task.Delay(TimeSpan.FromSeconds(config.DurationSecs), ct)
            : Task.Delay(Timeout.InfiniteTimeSpan, ct);

        var workers = Enumerable.Range(0, config.Concurrency)
            .Select(_ => WorkerAsync(ds, template, semaphore, ct))
            .ToArray();

        await Task.WhenAny(deadline, Task.WhenAll(workers)).ConfigureAwait(false);
        if (!ct.IsCancellationRequested && _cts is not null)
            await _cts.CancelAsync();

        if (!library || sp is null)
            await ds.DisposeAsync();
    }

    private async Task WorkerAsync(NpgsqlDataSource ds, ICockroachDBTemplate? template,
        SemaphoreSlim sem, CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            await sem.WaitAsync(ct);
            try
            {
                var op = Random.Shared.Next(4);
                var sw = System.Diagnostics.Stopwatch.StartNew();
                try
                {
                    if (template is not null)
                    {
                        await template.ExecuteAsync(async () =>
                        {
                            switch (op)
                            {
                                case 0: await DoInsertAsync(ds, ct); break;
                                case 1: await DoSelectAsync(ds, ct); break;
                                case 2: await DoUpdateAsync(ds, ct); break;
                                case 3: await DoDeleteAsync(ds, ct); break;
                            }
                        });
                    }
                    else
                    {
                        switch (op)
                        {
                            case 0: await DoInsertAsync(ds, ct); break;
                            case 1: await DoSelectAsync(ds, ct); break;
                            case 2: await DoUpdateAsync(ds, ct); break;
                            case 3: await DoDeleteAsync(ds, ct); break;
                        }
                    }
                    recorder.RecordOp(sw.ElapsedTicks / (TimeSpan.TicksPerMillisecond / 1000));
                }
                catch (OperationCanceledException) { return; }
                catch { recorder.RecordError(); }
            }
            finally { sem.Release(); }
        }
    }

    private static async Task DoInsertAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand(
            "INSERT INTO bench_accounts (id, balance, name, region) VALUES ($1, $2, $3, $4)");
        cmd.Parameters.AddWithValue(Guid.NewGuid());
        cmd.Parameters.AddWithValue((decimal)(1000 + Random.Shared.NextDouble() * 9000));
        cmd.Parameters.AddWithValue($"acct-{Random.Shared.Next(100_000)}");
        cmd.Parameters.AddWithValue(PickRegion());
        await cmd.ExecuteNonQueryAsync(ct);
    }

    private static async Task DoSelectAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand(
            "SELECT name, balance FROM bench_accounts WHERE id = $1");
        cmd.Parameters.AddWithValue(Guid.NewGuid());
        await using var reader = await cmd.ExecuteReaderAsync(ct);
        while (await reader.ReadAsync(ct)) { }
    }

    private static async Task DoUpdateAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand(
            "UPDATE bench_accounts SET balance = balance + $1, updated_at = now() WHERE id = $2");
        cmd.Parameters.AddWithValue((decimal)(Random.Shared.NextDouble() * 100 - 50));
        cmd.Parameters.AddWithValue(Guid.NewGuid());
        await cmd.ExecuteNonQueryAsync(ct);
    }

    private static async Task DoDeleteAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand(
            "DELETE FROM bench_accounts WHERE id = $1");
        cmd.Parameters.AddWithValue(Guid.NewGuid());
        await cmd.ExecuteNonQueryAsync(ct);
    }

    private static async Task EnsureSchemaAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand("""
            CREATE TABLE IF NOT EXISTS bench_accounts (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                balance DECIMAL(15,2) NOT NULL DEFAULT 1000.00,
                name STRING NOT NULL,
                region STRING NOT NULL DEFAULT 'us-east',
                created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
            )
            """);
        await cmd.ExecuteNonQueryAsync(ct);
    }

    private async Task TrackConnectionsAsync(NpgsqlDataSource ds, PeriodicTimer timer, CancellationToken ct)
    {
        while (await timer.WaitForNextTickAsync(ct))
        {
            try
            {
                await using var cmd = ds.CreateCommand("SELECT count(*) FROM crdb_internal.cluster_sessions WHERE active_queries != ''");
                var result = await cmd.ExecuteScalarAsync(ct);
                recorder.SetActiveConnections(Convert.ToInt32(result));
            }
            catch { }
        }
    }

    private static string PickRegion() => Random.Shared.Next(3) switch
    {
        0 => "us-east",
        1 => "us-west",
        _ => "eu-west"
    };
}
