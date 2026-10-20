using BenchmarkAgent.Protocol;

namespace BenchmarkAgent.Workloads;

public interface IWorkloadRunner
{
    Task StartAsync(WorkloadConfig config, CancellationToken ct);
    Task StopAsync();
    bool IsRunning { get; }
}
