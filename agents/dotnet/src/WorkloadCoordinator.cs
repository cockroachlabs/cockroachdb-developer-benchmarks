using BenchmarkAgent.Metrics;
using BenchmarkAgent.Protocol;
using BenchmarkAgent.Workloads;

namespace BenchmarkAgent;

public sealed class WorkloadCoordinator(MetricsRecorder recorder, string mode, IServiceProvider? serviceProvider = null)
{
    private readonly Lock _lock = new();
    private IWorkloadRunner? _runner;
    private CancellationTokenSource? _cts;
    private string _runId = "";
    private string _workloadType = "";

    public bool IsRunning
    {
        get { lock (_lock) { return _runner?.IsRunning == true; } }
    }

    public WorkloadResponse Start(WorkloadRequest request)
    {
        lock (_lock)
        {
            if (_runner?.IsRunning == true)
                return new WorkloadResponse(_runId, "already_running");

            _runId = Guid.NewGuid().ToString("N")[..12];
            _workloadType = request.Type;
            _cts = new CancellationTokenSource();

            recorder.Reset();
            recorder.SetContext(_runId, mode, _workloadType);

            _runner = WorkloadFactory.Create(request.Type, mode, recorder, serviceProvider);
            _ = _runner.StartAsync(request.Config, _cts.Token);

            return new WorkloadResponse(_runId, "started");
        }
    }

    public async Task<StopResponse> StopAsync()
    {
        IWorkloadRunner? runner;
        lock (_lock)
        {
            runner = _runner;
            _runner = null;
        }

        if (runner is not null)
        {
            await runner.StopAsync();
            _cts?.Dispose();
            _cts = null;
        }

        return new StopResponse("stopped");
    }
}
