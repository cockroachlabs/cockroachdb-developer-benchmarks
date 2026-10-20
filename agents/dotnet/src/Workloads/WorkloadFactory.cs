using BenchmarkAgent.Metrics;

namespace BenchmarkAgent.Workloads;

public static class WorkloadFactory
{
    public static IWorkloadRunner Create(string workloadType, string mode, MetricsRecorder recorder,
        IServiceProvider? serviceProvider = null)
    {
        var isLibrary = mode == "library";
        return workloadType switch
        {
            "crud" => new CrudWorkload(recorder, isLibrary, serviceProvider),
            "retry_storm" => new RetryStormWorkload(recorder, isLibrary, serviceProvider),
            "mixed_rw" => new MixedRWWorkload(recorder, isLibrary, serviceProvider),
            "follower_reads" => new FollowerReadsWorkload(recorder, isLibrary, serviceProvider),
            "cdc" => new CdcWorkload(recorder, isLibrary, serviceProvider),
            "vector_search" => new VectorSearchWorkload(recorder, isLibrary, serviceProvider),
            _ => throw new ArgumentException($"Unknown workload type: {workloadType}")
        };
    }
}
