using BenchmarkAgent.Metrics;
using BenchmarkAgent.Protocol;
using CockroachDB.DotNet.Abstractions.Cdc;
using CockroachDB.DotNet.Abstractions.Template;
using CockroachDB.DotNet.Cdc;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;
using Npgsql;

namespace BenchmarkAgent.Workloads;

public sealed class CdcWorkload(MetricsRecorder recorder, bool library, IServiceProvider? sp = null) : IWorkloadRunner
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
        ChangefeedManager? cfManager = null;

        if (library && sp is not null)
        {
            ds = sp.GetRequiredService<NpgsqlDataSource>();
            template = sp.GetRequiredService<ICockroachDBTemplate>();
            var logger = sp.GetService<ILogger<ChangefeedManager>>()
                         ?? NullLogger<ChangefeedManager>.Instance;
            cfManager = new ChangefeedManager(ds, logger);
        }
        else
        {
            var dsb = new NpgsqlDataSourceBuilder(config.DbUrl);
            dsb.ConnectionStringBuilder.MaxPoolSize = config.Concurrency * 2 + 5;
            ds = dsb.Build();
        }

        await EnsureSchemaAsync(ds, ct);

        var writerTask = WriterLoopAsync(ds, template, config.Concurrency, ct);
        var readerTask = library && cfManager is not null
            ? LibraryChangefeedReaderAsync(ds, cfManager, ct)
            : VanillaChangefeedReaderAsync(ds, ct);

        var deadline = config.DurationSecs > 0
            ? Task.Delay(TimeSpan.FromSeconds(config.DurationSecs), ct)
            : Task.Delay(Timeout.InfiniteTimeSpan, ct);

        await Task.WhenAny(deadline, Task.WhenAll(writerTask, readerTask)).ConfigureAwait(false);
        if (!ct.IsCancellationRequested && _cts is not null)
            await _cts.CancelAsync();

        if (!library || sp is null)
            await ds.DisposeAsync();
    }

    private async Task WriterLoopAsync(NpgsqlDataSource ds, ICockroachDBTemplate? template,
        int concurrency, CancellationToken ct)
    {
        var workers = Enumerable.Range(0, Math.Max(1, concurrency / 2))
            .Select(_ => WriteWorkerAsync(ds, template, ct))
            .ToArray();
        await Task.WhenAll(workers);
    }

    private async Task WriteWorkerAsync(NpgsqlDataSource ds, ICockroachDBTemplate? template,
        CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            var sw = System.Diagnostics.Stopwatch.StartNew();
            try
            {
                if (template is not null)
                {
                    await template.ExecuteAsync(async () =>
                    {
                        await DoInsertAsync(ds, ct);
                    });
                }
                else
                {
                    await DoInsertAsync(ds, ct);
                }
                recorder.RecordOp(sw.ElapsedTicks / (TimeSpan.TicksPerMillisecond / 1000));
            }
            catch (OperationCanceledException) { return; }
            catch { recorder.RecordError(); }
        }
    }

    private static async Task DoInsertAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand(
            "INSERT INTO bench_accounts (id, name, balance) VALUES ($1, $2, $3)");
        cmd.Parameters.AddWithValue(Random.Shared.NextInt64(100_000_000));
        cmd.Parameters.AddWithValue($"cdc-acct-{Random.Shared.Next(100_000)}");
        cmd.Parameters.AddWithValue((decimal)(Random.Shared.NextDouble() * 10000));
        await cmd.ExecuteNonQueryAsync(ct);
    }

    private async Task LibraryChangefeedReaderAsync(NpgsqlDataSource ds,
        ChangefeedManager cfManager, CancellationToken ct)
    {
        var cursor = "0";
        while (!ct.IsCancellationRequested)
        {
            try
            {
                await using var conn = await ds.OpenConnectionAsync(ct);
                await using var cmd = new NpgsqlCommand(
                    "EXPERIMENTAL CHANGEFEED FOR bench_accounts WITH updated, cursor = $1", conn);
                cmd.Parameters.AddWithValue(cursor);
                cmd.CommandTimeout = 5;

                await using var reader = await cmd.ExecuteReaderAsync(ct);
                while (await reader.ReadAsync(ct))
                {
                    var ts = reader.GetString(2);
                    if (!string.IsNullOrEmpty(ts)) cursor = ts;

                    var emitTime = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
                    await using var ins = ds.CreateCommand(
                        "INSERT INTO bench_cdc_events (source_table, operation, payload, lag_ms) VALUES ($1, $2, $3::JSONB, $4)");
                    ins.Parameters.AddWithValue("bench_accounts");
                    ins.Parameters.AddWithValue("changefeed");
                    ins.Parameters.AddWithValue(reader.GetString(1));
                    ins.Parameters.AddWithValue((int)(emitTime % 10_000));
                    await ins.ExecuteNonQueryAsync(ct);
                }
            }
            catch (OperationCanceledException) { return; }
            catch
            {
                recorder.RecordError();
                await Task.Delay(1000, ct);
            }
        }
    }

    private async Task VanillaChangefeedReaderAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        var cursor = "0";
        while (!ct.IsCancellationRequested)
        {
            try
            {
                await using var conn = await ds.OpenConnectionAsync(ct);
                await using var cmd = new NpgsqlCommand(
                    "EXPERIMENTAL CHANGEFEED FOR bench_accounts WITH cursor = $1", conn);
                cmd.Parameters.AddWithValue(cursor);
                cmd.CommandTimeout = 5;

                await using var reader = await cmd.ExecuteReaderAsync(ct);
                while (await reader.ReadAsync(ct))
                {
                    var ts = reader.GetString(2);
                    if (!string.IsNullOrEmpty(ts)) cursor = ts;

                    var emitTime = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
                    await using var ins = ds.CreateCommand(
                        "INSERT INTO bench_cdc_events (source_table, operation, payload, lag_ms) VALUES ($1, $2, $3::JSONB, $4)");
                    ins.Parameters.AddWithValue("bench_accounts");
                    ins.Parameters.AddWithValue("changefeed");
                    ins.Parameters.AddWithValue(reader.GetString(1));
                    ins.Parameters.AddWithValue((int)(emitTime % 10_000));
                    await ins.ExecuteNonQueryAsync(ct);
                }
            }
            catch (OperationCanceledException) { return; }
            catch
            {
                recorder.RecordError();
                await Task.Delay(1000, ct);
            }
        }
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
            CREATE TABLE IF NOT EXISTS bench_cdc_events (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                source_table STRING NOT NULL,
                operation STRING NOT NULL,
                payload JSONB NOT NULL,
                received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                lag_ms INT NOT NULL DEFAULT 0
            )
            """);
        await cmd2.ExecuteNonQueryAsync(ct);
    }
}
