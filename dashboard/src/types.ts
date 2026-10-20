export interface MetricSnapshot {
  ts: number;
  run_id: string;
  language: string;
  mode: string;
  workload: string;
  ops_total: number;
  ops_per_sec: number;
  latency_p50_us: number;
  latency_p95_us: number;
  latency_p99_us: number;
  latency_max_us: number;
  errors: number;
  retries: number;
  retry_success: number;
  circuit_breaker_open: boolean;
  active_connections: number;
  cpu_percent: number;
  mem_rss_mb: number;
  error_samples?: { sqlstate: string; message: string; count: number }[];
}

export interface ChaosEvent {
  timestamp: string;
  scenario: string;
  action: string;
  target: string;
  duration_ms: number;
}

export interface AgentEntry {
  language: string;
  mode: string;
  port: number;
  host: string;
}

export interface BenchmarkRun {
  id: string;
  timestamp: string;
  topology: Topology;
  workloads: WorkloadType[];
  duration_secs: number;
  agents: AgentEntry[];
  chaos_events?: ChaosEvent[];
}

export type WorkloadType = 'crud' | 'retry_storm' | 'mixed_rw' | 'follower_reads' | 'cdc' | 'vector_search';

export type Topology = 'single' | 'cluster' | 'multi';

export type ChaosMode = 'individual' | 'gauntlet' | 'random';

export type MetricKey =
  | 'ops_per_sec'
  | 'latency_p50_us'
  | 'latency_p95_us'
  | 'latency_p99_us'
  | 'latency_max_us'
  | 'errors'
  | 'retries'
  | 'cpu_percent'
  | 'mem_rss_mb';

export const WORKLOAD_TYPES: WorkloadType[] = [
  'crud', 'retry_storm', 'mixed_rw', 'follower_reads', 'cdc', 'vector_search',
];

export const WORKLOAD_LABELS: Record<WorkloadType, string> = {
  crud: 'CRUD',
  retry_storm: 'Retry Storm',
  mixed_rw: 'Mixed R/W',
  follower_reads: 'Follower Reads',
  cdc: 'CDC',
  vector_search: 'Vector Search',
};

export interface WorkloadInfo {
  description: string;
  measures: string;
  libraryAdvantage: string;
  expect: string;
}

export const WORKLOAD_INFO: Record<WorkloadType, WorkloadInfo> = {
  crud: {
    description: 'Basic INSERT, SELECT, UPDATE, DELETE operations on a single table. Measures raw throughput and per-operation latency.',
    measures: 'Overhead cost of the library middleware pipeline on simple operations.',
    libraryAdvantage: 'Connection pool optimization, error classification. Expect 2-10% overhead — the price of safety.',
    expect: 'Vanilla should be slightly faster. This workload measures the cost, not the benefit, of using the library.',
  },
  retry_storm: {
    description: 'High-contention writes on 10 hot rows with 32 concurrent workers, forcing SQLSTATE 40001 serialization conflicts.',
    measures: 'Retry handling under contention — the core value proposition of the libraries.',
    libraryAdvantage: 'Exponential backoff + jitter prevents thundering herd. Python uses server-side savepoint restart. Libraries should show 3-5x fewer failed transactions.',
    expect: 'Library agents should have dramatically fewer errors and higher retry success rates. This is where libraries pay for themselves.',
  },
  mixed_rw: {
    description: '80% reads / 20% writes with JOINs across bench_accounts and bench_orders tables. Realistic production-like workload mix.',
    measures: 'Overall middleware overhead under realistic conditions with mixed transaction types.',
    libraryAdvantage: 'Transaction management, read/write routing, session handling. Moderate overhead expected.',
    expect: 'Similar to CRUD but with more complex queries. Library overhead should be minimal on reads.',
  },
  follower_reads: {
    description: 'Read queries using AS OF SYSTEM TIME follower_read_timestamp() to route to the nearest replica instead of the leaseholder.',
    measures: 'Read latency reduction by routing to followers. Requires 3+ node cluster.',
    libraryAdvantage: 'Library automatically injects follower read SQL. No code changes needed. Expect 30-60% lower read latency on multi-node.',
    expect: 'On single-node: no difference (no followers). On 3+ nodes: library reads should be significantly faster.',
  },
  cdc: {
    description: 'Continuous writes with a CockroachDB changefeed consumer measuring event processing rate and lag.',
    measures: 'CDC event processing throughput and end-to-end event lag (write to receipt).',
    libraryAdvantage: 'Typed event handlers, structured webhook parsing, automatic changefeed creation. Cleaner code with minimal performance difference.',
    expect: 'Similar throughput. The library value here is developer experience, not raw performance.',
  },
  vector_search: {
    description: 'INSERT 1536-dimension vectors and run cosine similarity searches against bench_documents table.',
    measures: 'Vector indexing speed and search latency using CockroachDB native VECTOR type.',
    libraryAdvantage: 'VectorSearchTemplate provides clean API vs raw SQL. Less than 1ms overhead per query.',
    expect: 'Nearly identical performance. The library abstracts the SQL but adds negligible overhead.',
  },
};

export const LANGUAGE_COLORS: Record<string, string> = {
  java: '#4ecdc4',
  'java-native': '#2ecc71',
  go: '#ff6b6b',
  python: '#ffd93d',
  dotnet: '#c084fc',
};

export const AGENTS = [
  { language: 'java', mode: 'library', label: 'Java Lib', color: '#4ecdc4' },
  { language: 'java', mode: 'vanilla', label: 'Java Raw', color: '#ff9f43' },
  { language: 'java-native', mode: 'library', label: 'Java Native Lib', color: '#2ecc71' },
  { language: 'java-native', mode: 'vanilla', label: 'Java Native Raw', color: '#0abde3' },
  { language: 'go', mode: 'library', label: 'Go Lib', color: '#ff6b6b' },
  { language: 'go', mode: 'vanilla', label: 'Go Raw', color: '#feca57' },
  { language: 'python', mode: 'library', label: 'Python Lib', color: '#a29bfe' },
  { language: 'python', mode: 'vanilla', label: 'Python Raw', color: '#fd79a8' },
  { language: 'dotnet', mode: 'library', label: '.NET Lib', color: '#00cec9' },
  { language: 'dotnet', mode: 'vanilla', label: '.NET Raw', color: '#e17055' },
];

export type MetricDirection = 'higher' | 'lower';

export interface MetricInfo {
  value: MetricKey;
  label: string;
  direction: MetricDirection;
  unit: string;
}

export const METRIC_OPTIONS: MetricInfo[] = [
  { value: 'ops_per_sec', label: 'Throughput', direction: 'higher', unit: 'ops/s' },
  { value: 'latency_p50_us', label: 'P50 Latency', direction: 'lower', unit: 'us' },
  { value: 'latency_p95_us', label: 'P95 Latency', direction: 'lower', unit: 'us' },
  { value: 'latency_p99_us', label: 'P99 Latency', direction: 'lower', unit: 'us' },
  { value: 'latency_max_us', label: 'Max Latency', direction: 'lower', unit: 'us' },
  { value: 'errors', label: 'Errors', direction: 'lower', unit: '' },
  { value: 'retries', label: 'Retries', direction: 'lower', unit: '' },
  { value: 'cpu_percent', label: 'CPU', direction: 'lower', unit: '%' },
  { value: 'mem_rss_mb', label: 'Memory', direction: 'lower', unit: 'MB' },
];

export const DEFAULT_VISIBLE_METRICS: MetricKey[] = [
  'ops_per_sec', 'latency_p99_us', 'errors', 'retries',
];

export function getMetricInfo(key: MetricKey): MetricInfo {
  return METRIC_OPTIONS.find(m => m.value === key)!;
}

export interface TrendPoint {
  run_id: string;
  timestamp: string;
  language: string;
  mode: string;
  workload: string;
  avg_ops: number;
}

export interface RegressionItem {
  agent: string;
  workload: string;
  metric: string;
  previous_value: number;
  current_value: number;
  change_pct: number;
  is_regression: boolean;
}

export function agentKey(language: string, mode: string): string {
  return `${language}-${mode}`;
}
