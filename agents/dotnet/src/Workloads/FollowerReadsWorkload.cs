using BenchmarkAgent.Metrics;
using BenchmarkAgent.Protocol;
using CockroachDB.DotNet.Abstractions.Template;
using Microsoft.Extensions.DependencyInjection;
using Npgsql;

namespace BenchmarkAgent.Workloads;

public sealed class FollowerReadsWorkload(MetricsRecorder recorder, bool library, IServiceProvider? sp = null) : IWorkloadRunner
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
        await SeedDataAsync(ds, ct);

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
            var sw = System.Diagnostics.Stopwatch.StartNew();
            try
            {
                if (template is not null)
                    await DoLibraryReadAsync(ds, template, ct);
                else
                    await DoVanillaReadAsync(ds, ct);

                recorder.RecordOp(sw.ElapsedTicks / (TimeSpan.TicksPerMillisecond / 1000));
            }
            catch (OperationCanceledException) { return; }
            catch { recorder.RecordError(); }
        }
    }

    private static async Task DoLibraryReadAsync(NpgsqlDataSource ds, ICockroachDBTemplate template,
        CancellationToken ct)
    {
        await template.ExecuteFollowerReadAsync(async () =>
        {
            await using var cmd = ds.CreateCommand(
                "SELECT name, balance, region FROM bench_accounts WHERE id = $1");
            cmd.Parameters.AddWithValue(Random.Shared.NextInt64(1000) + 1);
            await using var reader = await cmd.ExecuteReaderAsync(ct);
            while (await reader.ReadAsync(ct)) { }
        });
    }

    private static async Task DoVanillaReadAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand(
            "SELECT name, balance, region FROM bench_accounts AS OF SYSTEM TIME '-5s' WHERE id = $1");
        cmd.Parameters.AddWithValue(Random.Shared.NextInt64(1000) + 1);
        await using var reader = await cmd.ExecuteReaderAsync(ct);
        while (await reader.ReadAsync(ct)) { }
    }

    private static async Task EnsureSchemaAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand("""
            CREATE TABLE IF NOT EXISTS bench_accounts (
                id INT8 PRIMARY KEY,
                name STRING NOT NULL,
                balance DECIMAL(15,2) NOT NULL DEFAULT 0,
                region STRING NOT NULL DEFAULT 'us-east',
                created_at TIMESTAMPTZ NOT NULL DEFAULT now()
            )
            """);
        await cmd.ExecuteNonQueryAsync(ct);
    }

    private static async Task SeedDataAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        for (var i = 1; i <= 1000; i++)
        {
            var region = (i % 3) switch { 0 => "us-east", 1 => "us-west", _ => "eu-west" };
            await using var cmd = ds.CreateCommand(
                "UPSERT INTO bench_accounts (id, name, balance, region) VALUES ($1, $2, $3, $4)");
            cmd.Parameters.AddWithValue((long)i);
            cmd.Parameters.AddWithValue($"account-{i}");
            cmd.Parameters.AddWithValue(10000m);
            cmd.Parameters.AddWithValue(region);
            await cmd.ExecuteNonQueryAsync(ct);
        }
    }
}
