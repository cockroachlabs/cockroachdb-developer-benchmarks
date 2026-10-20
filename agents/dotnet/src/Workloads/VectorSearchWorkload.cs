using System.Text;
using BenchmarkAgent.Metrics;
using BenchmarkAgent.Protocol;
using CockroachDB.DotNet.AI;
using CockroachDB.DotNet.AI.Metrics;
using CockroachDB.DotNet.AI.Template;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;
using Npgsql;

namespace BenchmarkAgent.Workloads;

public sealed class VectorSearchWorkload(MetricsRecorder recorder, bool library, IServiceProvider? sp = null) : IWorkloadRunner
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
        VectorSearchTemplate? vsTemplate = null;

        if (library && sp is not null)
        {
            ds = sp.GetRequiredService<NpgsqlDataSource>();
            var metrics = sp.GetService<IAIMetrics>() ?? NoOpAIMetrics.Instance;
            var logger = sp.GetService<ILogger<VectorSearchTemplate>>()
                         ?? NullLogger<VectorSearchTemplate>.Instance;
            vsTemplate = new VectorSearchTemplate(ds, metrics, logger);
        }
        else
        {
            var dsb = new NpgsqlDataSourceBuilder(config.DbUrl);
            dsb.ConnectionStringBuilder.MaxPoolSize = config.Concurrency * 2;
            ds = dsb.Build();
        }

        await EnsureSchemaAsync(ds, ct);

        var workers = Enumerable.Range(0, config.Concurrency)
            .Select(_ => WorkerAsync(ds, vsTemplate, ct))
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

    private async Task WorkerAsync(NpgsqlDataSource ds, VectorSearchTemplate? vsTemplate,
        CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            var isInsert = Random.Shared.NextDouble() < 0.3;
            var sw = System.Diagnostics.Stopwatch.StartNew();
            try
            {
                if (isInsert)
                {
                    if (vsTemplate is not null)
                        await DoLibraryInsertAsync(vsTemplate, ct);
                    else
                        await DoVanillaInsertAsync(ds, ct);
                }
                else
                {
                    if (vsTemplate is not null)
                        await DoLibrarySearchAsync(vsTemplate, ct);
                    else
                        await DoVanillaSearchAsync(ds, ct);
                }

                recorder.RecordOp(sw.ElapsedTicks / (TimeSpan.TicksPerMillisecond / 1000));
            }
            catch (OperationCanceledException) { return; }
            catch { recorder.RecordError(); }
        }
    }

    private static async Task DoLibraryInsertAsync(VectorSearchTemplate vsTemplate, CancellationToken ct)
    {
        var embedding = GenerateRandomVector(1536);
        var docId = Guid.NewGuid();
        var metadata = new Dictionary<string, object?>
        {
            ["content"] = $"doc-{Random.Shared.Next(1_000_000)}",
            ["metadata"] = $"{{\"category\":\"bench\",\"score\":{Random.Shared.NextDouble():F4}}}"
        };
        await vsTemplate.StoreEmbeddingWithMetadataAsync(
            "bench_documents", "id", docId, "embedding", embedding, metadata, ct);
    }

    private static async Task DoLibrarySearchAsync(VectorSearchTemplate vsTemplate, CancellationToken ct)
    {
        var queryVec = GenerateRandomVector(1536);
        await vsTemplate.SearchAsync(
            "bench_documents", "embedding", queryVec, DistanceMetric.Cosine, 10, ct);
    }

    private static async Task DoVanillaInsertAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        var embedding = GenerateRandomVector(1536);
        await using var cmd = ds.CreateCommand(
            "INSERT INTO bench_documents (content, embedding, metadata) VALUES ($1, $2, $3::JSONB)");
        cmd.Parameters.AddWithValue($"doc-{Random.Shared.Next(1_000_000)}");
        cmd.Parameters.AddWithValue(FormatVector(embedding));
        cmd.Parameters.AddWithValue($"{{\"category\":\"bench\",\"score\":{Random.Shared.NextDouble():F4}}}");
        await cmd.ExecuteNonQueryAsync(ct);
    }

    private static async Task DoVanillaSearchAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        var queryVec = GenerateRandomVector(1536);
        await using var cmd = ds.CreateCommand("""
            SELECT id, content, embedding <=> $1 AS distance
            FROM bench_documents
            ORDER BY embedding <=> $1
            LIMIT 10
            """);
        cmd.Parameters.AddWithValue(FormatVector(queryVec));
        await using var reader = await cmd.ExecuteReaderAsync(ct);
        while (await reader.ReadAsync(ct)) { }
    }

    private static float[] GenerateRandomVector(int dims)
    {
        var vec = new float[dims];
        for (var i = 0; i < dims; i++)
            vec[i] = (float)(Random.Shared.NextDouble() * 2 - 1);
        return vec;
    }

    private static string FormatVector(float[] vec)
    {
        var sb = new StringBuilder("[");
        for (var i = 0; i < vec.Length; i++)
        {
            if (i > 0) sb.Append(',');
            sb.Append(vec[i].ToString("G9"));
        }
        sb.Append(']');
        return sb.ToString();
    }

    private static async Task EnsureSchemaAsync(NpgsqlDataSource ds, CancellationToken ct)
    {
        await using var cmd = ds.CreateCommand("""
            CREATE TABLE IF NOT EXISTS bench_documents (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                content STRING NOT NULL,
                embedding VECTOR(1536),
                metadata JSONB,
                created_at TIMESTAMPTZ NOT NULL DEFAULT now()
            )
            """);
        await cmd.ExecuteNonQueryAsync(ct);
    }
}
