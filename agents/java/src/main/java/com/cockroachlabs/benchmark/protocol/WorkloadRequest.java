package com.cockroachlabs.benchmark.protocol;

import com.fasterxml.jackson.annotation.JsonProperty;

public record WorkloadRequest(
        String type,
        String mode,
        WorkloadConfig config
) {
    public record WorkloadConfig(
            @JsonProperty("duration_secs") Integer durationSecs,
            Integer concurrency,
            @JsonProperty("db_url") String dbUrl,
            @JsonProperty("hot_rows") Integer hotRows
    ) {
        public WorkloadConfig {
            if (durationSecs == null) durationSecs = 30;
            if (concurrency == null) concurrency = 8;
            if (hotRows == null) hotRows = 0;
        }
    }
}
