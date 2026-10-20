using System.Text.Json.Serialization;

namespace BenchmarkAgent.Protocol;

public record WorkloadRequest(
    [property: JsonPropertyName("type")] string Type,
    [property: JsonPropertyName("mode")] string Mode,
    [property: JsonPropertyName("config")] WorkloadConfig Config
);

public record WorkloadConfig(
    [property: JsonPropertyName("duration_secs")] int DurationSecs,
    [property: JsonPropertyName("concurrency")] int Concurrency,
    [property: JsonPropertyName("db_url")] string DbUrl,
    [property: JsonPropertyName("hot_rows")] int HotRows = 0
);

public record WorkloadResponse(
    [property: JsonPropertyName("run_id")] string RunId,
    [property: JsonPropertyName("status")] string Status
);

public record StopResponse(
    [property: JsonPropertyName("status")] string Status
);

public record HealthResponse(
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("language")] string Language,
    [property: JsonPropertyName("mode")] string Mode
);

public record AgentInfo(
    [property: JsonPropertyName("language")] string Language,
    [property: JsonPropertyName("library_version")] string LibraryVersion,
    [property: JsonPropertyName("driver")] string Driver
);
