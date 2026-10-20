import { describe, it, expect } from 'vitest';
import {
  agentKey,
  getMetricInfo,
  AGENTS,
  WORKLOAD_INFO,
  WORKLOAD_TYPES,
  METRIC_OPTIONS,
  type MetricKey,
  type WorkloadType,
} from '../types';

describe('agentKey', () => {
  it('produces correct keys', () => {
    expect(agentKey('go', 'library')).toBe('go-library');
    expect(agentKey('java', 'vanilla')).toBe('java-vanilla');
    expect(agentKey('python', 'library')).toBe('python-library');
    expect(agentKey('dotnet', 'vanilla')).toBe('dotnet-vanilla');
    expect(agentKey('java-native', 'library')).toBe('java-native-library');
  });
});

describe('getMetricInfo', () => {
  it('returns correct direction for throughput', () => {
    const info = getMetricInfo('ops_per_sec');
    expect(info.direction).toBe('higher');
    expect(info.label).toBe('Throughput');
    expect(info.unit).toBe('ops/s');
  });

  it('returns correct direction for latency metrics', () => {
    const latencyKeys: MetricKey[] = [
      'latency_p50_us',
      'latency_p95_us',
      'latency_p99_us',
      'latency_max_us',
    ];
    for (const key of latencyKeys) {
      const info = getMetricInfo(key);
      expect(info.direction).toBe('lower');
    }
  });

  it('returns correct direction for errors and retries', () => {
    expect(getMetricInfo('errors').direction).toBe('lower');
    expect(getMetricInfo('retries').direction).toBe('lower');
  });

  it('returns correct direction for resource metrics', () => {
    expect(getMetricInfo('cpu_percent').direction).toBe('lower');
    expect(getMetricInfo('mem_rss_mb').direction).toBe('lower');
  });

  it('covers all MetricKey values', () => {
    const allKeys: MetricKey[] = [
      'ops_per_sec',
      'latency_p50_us',
      'latency_p95_us',
      'latency_p99_us',
      'latency_max_us',
      'errors',
      'retries',
      'cpu_percent',
      'mem_rss_mb',
    ];
    for (const key of allKeys) {
      const info = getMetricInfo(key);
      expect(info).toBeDefined();
      expect(info.value).toBe(key);
    }
  });
});

describe('AGENTS', () => {
  it('has unique colors', () => {
    const colors = AGENTS.map((a) => a.color);
    const unique = new Set(colors);
    expect(unique.size).toBe(colors.length);
  });

  it('has unique language-mode combinations', () => {
    const keys = AGENTS.map((a) => agentKey(a.language, a.mode));
    const unique = new Set(keys);
    expect(unique.size).toBe(keys.length);
  });

  it('contains expected languages', () => {
    const languages = new Set(AGENTS.map((a) => a.language));
    expect(languages.has('java')).toBe(true);
    expect(languages.has('go')).toBe(true);
    expect(languages.has('python')).toBe(true);
    expect(languages.has('dotnet')).toBe(true);
  });

  it('has both library and vanilla modes per language', () => {
    const languages = [...new Set(AGENTS.map((a) => a.language))];
    for (const lang of languages) {
      const modes = AGENTS.filter((a) => a.language === lang).map((a) => a.mode);
      expect(modes).toContain('library');
      expect(modes).toContain('vanilla');
    }
  });
});

describe('WORKLOAD_INFO', () => {
  it('has entries for all WORKLOAD_TYPES', () => {
    for (const wt of WORKLOAD_TYPES) {
      const info = WORKLOAD_INFO[wt];
      expect(info).toBeDefined();
      expect(info.description.length).toBeGreaterThan(0);
      expect(info.measures.length).toBeGreaterThan(0);
      expect(info.libraryAdvantage.length).toBeGreaterThan(0);
      expect(info.expect.length).toBeGreaterThan(0);
    }
  });

  it('covers exactly 6 workload types', () => {
    expect(WORKLOAD_TYPES.length).toBe(6);
    const expected: WorkloadType[] = [
      'crud',
      'retry_storm',
      'mixed_rw',
      'follower_reads',
      'cdc',
      'vector_search',
    ];
    for (const wt of expected) {
      expect(WORKLOAD_TYPES).toContain(wt);
    }
  });
});

describe('METRIC_OPTIONS', () => {
  it('has unique values', () => {
    const values = METRIC_OPTIONS.map((m) => m.value);
    const unique = new Set(values);
    expect(unique.size).toBe(values.length);
  });

  it('only ops_per_sec has higher direction', () => {
    const higher = METRIC_OPTIONS.filter((m) => m.direction === 'higher');
    expect(higher.length).toBe(1);
    expect(higher[0].value).toBe('ops_per_sec');
  });
});
