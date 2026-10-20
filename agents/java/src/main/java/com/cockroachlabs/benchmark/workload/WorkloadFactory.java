package com.cockroachlabs.benchmark.workload;

import com.cockroachlabs.data.ai.VectorSearchTemplate;
import com.cockroachlabs.data.template.CockroachDBTemplate;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import javax.sql.DataSource;

@Component
public class WorkloadFactory {

    private final DataSource dataSource;
    private final String mode;
    private final CockroachDBTemplate cockroachDBTemplate;
    private final VectorSearchTemplate vectorSearchTemplate;

    public WorkloadFactory(DataSource dataSource,
                           @Value("${agent.mode}") String mode,
                           CockroachDBTemplate cockroachDBTemplate,
                           VectorSearchTemplate vectorSearchTemplate) {
        this.dataSource = dataSource;
        this.mode = mode;
        this.cockroachDBTemplate = cockroachDBTemplate;
        this.vectorSearchTemplate = vectorSearchTemplate;
    }

    public WorkloadRunner create(String workloadType) {
        return switch (workloadType) {
            case "crud" -> new CrudWorkload(dataSource, mode, cockroachDBTemplate);
            case "retry_storm" -> new RetryStormWorkload(dataSource, mode, cockroachDBTemplate);
            case "mixed_rw" -> new MixedRWWorkload(dataSource, mode, cockroachDBTemplate);
            case "follower_reads" -> new FollowerReadsWorkload(dataSource, mode, cockroachDBTemplate);
            case "cdc" -> new CdcWorkload(dataSource, mode, cockroachDBTemplate);
            case "vector_search" -> new VectorSearchWorkload(dataSource, mode, vectorSearchTemplate);
            default -> throw new IllegalArgumentException("Unknown workload type: " + workloadType);
        };
    }

    public String getMode() { return mode; }
}
