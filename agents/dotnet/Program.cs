using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using BenchmarkAgent;
using BenchmarkAgent.Metrics;
using BenchmarkAgent.Protocol;
using CockroachDB.DotNet.Abstractions.Configuration;
using CockroachDB.DotNet.Core.Extensions;
using Microsoft.Extensions.Options;
using Npgsql;

static string ConvertToNpgsql(string url)
{
    if (!url.StartsWith("postgresql://")) return url;
    var uri = new Uri(url);
    var db = uri.AbsolutePath.TrimStart('/');
    var host = uri.Host;
    var port = uri.Port > 0 ? uri.Port : 26257;
    var user = uri.UserInfo;
    return $"Host={host};Port={port};Database={db};Username={user};SSL Mode=Disable";
}

var mode = Environment.GetEnvironmentVariable("AGENT_MODE") ?? "vanilla";
var port = Environment.GetEnvironmentVariable("AGENT_PORT")
           ?? (mode == "library" ? "7007" : "7008");

var recorder = new MetricsRecorder();

var builder = WebApplication.CreateSlimBuilder(args);
builder.WebHost.UseUrls($"http://0.0.0.0:{port}");
builder.Services.ConfigureHttpJsonOptions(opts =>
    opts.SerializerOptions.PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower);

if (mode == "library")
{
    var dbUrl = ConvertToNpgsql(Environment.GetEnvironmentVariable("DB_URL")
                ?? "postgresql://root@localhost:26257/benchmark?sslmode=disable");

    builder.Services.AddSingleton(new NpgsqlDataSourceBuilder(dbUrl).Build());

    builder.Services.Configure<CockroachDBOptions>(opts =>
    {
        opts.Retry.MaxRetries = 10;
        opts.Retry.DelayMs = 50;
        opts.Retry.Multiplier = 2.0;
        opts.Retry.MaxDelayMs = 5000;
        opts.Retry.JitterMs = 25;
        opts.CircuitBreaker.FailureThreshold = 10;
        opts.CircuitBreaker.DurationOfBreakSeconds = 15;
    });

    builder.Services.AddCockroachDB(builder.Configuration, pipeline =>
    {
        pipeline.UseMetrics();
        pipeline.UseCircuitBreaker();
        pipeline.UseRetry();
        pipeline.UseFollowerRead();
    });
}

builder.Services.AddSingleton(recorder);

var app = builder.Build();
app.UseWebSockets();

var coordinator = new WorkloadCoordinator(recorder, mode,
    mode == "library" ? app.Services : null);


app.MapPost("/workload", (WorkloadRequest request) =>
{
    var rawUrl = string.IsNullOrEmpty(request.Config.DbUrl)
        ? Environment.GetEnvironmentVariable("DB_URL")
          ?? "postgresql://root@localhost:26257/benchmark?sslmode=disable"
        : request.Config.DbUrl;
    var config = request.Config with { DbUrl = ConvertToNpgsql(rawUrl) };

    var response = coordinator.Start(request with { Config = config });
    return Results.Ok(response);
});

app.MapPost("/stop", async () =>
{
    var response = await coordinator.StopAsync();
    return Results.Ok(response);
});

app.MapGet("/health", () => Results.Ok(new HealthResponse(
    Status: coordinator.IsRunning ? "running" : "idle",
    Language: "dotnet",
    Mode: mode
)));

app.MapGet("/info", () => Results.Ok(new AgentInfo(
    Language: "dotnet",
    LibraryVersion: mode == "library" ? "cockroachdb-dotnet-1.0" : "npgsql-9.0",
    Driver: mode == "library" ? "CockroachDB.DotNet.Core+Pipeline" : "Npgsql-raw"
)));

app.Map("/metrics", async (HttpContext ctx) =>
{
    if (!ctx.WebSockets.IsWebSocketRequest)
    {
        ctx.Response.StatusCode = 400;
        await ctx.Response.WriteAsync("WebSocket required");
        return;
    }

    using var ws = await ctx.WebSockets.AcceptWebSocketAsync();
    using var timer = new PeriodicTimer(TimeSpan.FromSeconds(1));

    while (await timer.WaitForNextTickAsync(ctx.RequestAborted))
    {
        if (ws.State != WebSocketState.Open) break;

        var snap = recorder.Snapshot();
        var json = JsonSerializer.SerializeToUtf8Bytes(snap);
        await ws.SendAsync(json, WebSocketMessageType.Text, true, ctx.RequestAborted);
    }

    if (ws.State == WebSocketState.Open)
        await ws.CloseAsync(WebSocketCloseStatus.NormalClosure, "done", CancellationToken.None);
});

Console.WriteLine($".NET benchmark agent starting mode={mode} port={port}");
app.Run();
