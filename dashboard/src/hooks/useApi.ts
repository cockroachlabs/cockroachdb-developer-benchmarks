import { useCallback } from 'react';
import { type BenchmarkRun, type MetricSnapshot, type WorkloadType, type Topology, type ChaosMode, type ChaosEvent, type TrendPoint, type RegressionItem } from '../types';

export interface ScenarioListItem {
  name: string;
  description: string;
  source: string;
  duration_secs: number;
  operations: number;
}

interface StartParams {
  workloads: WorkloadType[];
  topology: Topology;
  duration_secs: number;
  warmup_secs?: number;
  iterations?: number;
  chaos_mode?: ChaosMode;
  languages?: string[];
  isolation?: boolean;
}

interface StatusResponse {
  status: string;
  topology: string;
  topo_running: boolean;
  benchmark: boolean;
  chaos: boolean;
  subscribers: number;
  current_run?: { id: string; started_at: string };
}

interface RunDetail {
  run: BenchmarkRun;
  metrics: MetricSnapshot[];
}

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(path, opts);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

export function useApi() {
  const startBenchmark = useCallback(async (params: StartParams) => {
    return request<{ run_id: string; status: string }>('/api/benchmark/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
  }, []);

  const stopBenchmark = useCallback(async () => {
    return request<{ status: string }>('/api/benchmark/stop', { method: 'POST' });
  }, []);

  const getRuns = useCallback(async () => {
    return request<BenchmarkRun[]>('/api/runs');
  }, []);

  const getRun = useCallback(async (id: string) => {
    return request<RunDetail>(`/api/runs/${id}`);
  }, []);

  const getStatus = useCallback(async () => {
    return request<StatusResponse>('/api/status');
  }, []);

  const exportJSON = useCallback(async (runId: string) => {
    window.open(`/api/runs/${runId}/export`, '_blank');
  }, []);

  const exportHTML = useCallback(async (runId: string) => {
    window.open(`/api/runs/${runId}/report`, '_blank');
  }, []);

  const downloadHTML = useCallback(async (runId: string) => {
    window.open(`/api/runs/${runId}/report/download`, '_blank');
  }, []);

  const deleteRun = useCallback(async (id: string) => {
    return request<{ status: string }>(`/api/runs/${id}`, { method: 'DELETE' });
  }, []);

  const deleteAllRuns = useCallback(async () => {
    return request<{ status: string }>('/api/runs', { method: 'DELETE' });
  }, []);

  const teardown = useCallback(async () => {
    return request<Record<string, string>>('/api/teardown', { method: 'POST' });
  }, []);

  const compareReport = useCallback(async (runIds: string[]) => {
    const res = await fetch('/api/runs/compare/report', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ run_ids: runIds }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || res.statusText);
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    window.open(url, '_blank');
  }, []);

  const startMonitoring = useCallback(async () => {
    return request<{ status: string }>('/api/monitoring/start', { method: 'POST' });
  }, []);

  const stopMonitoring = useCallback(async () => {
    return request<{ status: string }>('/api/monitoring/stop', { method: 'POST' });
  }, []);

  const triggerChaos = useCallback(async (scenario: string, target?: string) => {
    return request<{ status: string; scenario: string; target: string }>('/api/chaos/trigger', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ scenario, target: target || '' }),
    });
  }, []);

  const getChaosStatus = useCallback(async () => {
    return request<{ running: boolean; events: ChaosEvent[] }>('/api/chaos/status');
  }, []);

  const getChaosPlans = useCallback(async () => {
    return request<{ name: string; description: string; events: unknown[] }[]>('/api/chaos/plans');
  }, []);

  const runChaosPlan = useCallback(async (plan: string) => {
    return request<{ status: string; plan: string }>('/api/chaos/plan', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ plan }),
    });
  }, []);

  const getTrends = useCallback(async (limit = 10) => {
    return request<TrendPoint[]>(`/api/trends?limit=${limit}`);
  }, []);

  const getRegressions = useCallback(async () => {
    return request<RegressionItem[]>('/api/regression');
  }, []);

  const getScenarios = useCallback(async () => {
    return request<ScenarioListItem[]>('/api/scenarios');
  }, []);

  const runScenario = useCallback(async (name: string) => {
    return request<{ run_id: string; status: string; scenario: string }>('/api/scenario/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ scenario: name }),
    });
  }, []);

  const uploadScenario = useCallback(async (file: File) => {
    const text = await file.text();
    return request<{ status: string; scenario: string; file: string }>('/api/scenarios/upload', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: text,
    });
  }, []);

  return { startBenchmark, stopBenchmark, getRuns, getRun, getStatus, exportJSON, exportHTML, downloadHTML, teardown, deleteRun, deleteAllRuns, compareReport, startMonitoring, stopMonitoring, triggerChaos, getChaosStatus, getChaosPlans, runChaosPlan, getTrends, getRegressions, getScenarios, runScenario, uploadScenario };
}
