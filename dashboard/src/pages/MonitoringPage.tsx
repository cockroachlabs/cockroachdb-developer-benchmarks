import { useState } from 'react'
import { useApi } from '../hooks/useApi'

export function MonitoringPage() {
  const { startMonitoring, stopMonitoring } = useApi();
  const [prometheusPort, setPrometheusPort] = useState(9090);
  const [grafanaPort, setGrafanaPort] = useState(3001);
  const [prometheusStatus, setPrometheusStatus] = useState<string>('unknown');
  const [launching, setLaunching] = useState(false);
  const [grafanaStatus, setGrafanaStatus] = useState<string>('unknown');

  const promUrl = `http://localhost:${prometheusPort}`;
  const grafUrl = `http://localhost:${grafanaPort}`;

  const checkStatus = async () => {
    try {
      const r = await fetch(`${promUrl}/-/healthy`);
      setPrometheusStatus(r.ok ? 'running' : 'down');
    } catch { setPrometheusStatus('down'); }
    try {
      const r = await fetch(`${grafUrl}/api/health`);
      setGrafanaStatus(r.ok ? 'running' : 'down');
    } catch { setGrafanaStatus('down'); }
  };

  const statusDot = (s: string) => ({
    width: 8, height: 8, borderRadius: '50%',
    background: s === 'running' ? '#4ecdc4' : s === 'down' ? '#ff6b6b' : '#555',
    display: 'inline-block',
  });

  return (
    <div style={{ paddingBottom: 220 }}>
      <div style={{ fontSize: 18, fontWeight: 600, marginBottom: 4 }}>Prometheus & Grafana</div>
      <div style={{ fontSize: 13, color: '#888', marginBottom: 12 }}>
        Monitor benchmark metrics with Prometheus and visualize in Grafana.
      </div>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', marginBottom: 20, fontSize: 12 }}>
        <span style={{ color: '#888' }}>Prometheus Port</span>
        <input type="number" value={prometheusPort} onChange={e => setPrometheusPort(Number(e.target.value))} style={{ width: 70 }} />
        <span style={{ color: '#888', marginLeft: 8 }}>Grafana Port</span>
        <input type="number" value={grafanaPort} onChange={e => setGrafanaPort(Number(e.target.value))} style={{ width: 70 }} />
      </div>

      <div style={{
        display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 24,
      }}>
        <div style={{
          background: 'var(--bg-card)', border: '1px solid var(--border)',
          borderRadius: 8, padding: 20,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
            <span style={statusDot(prometheusStatus)} />
            <span style={{ fontSize: 15, fontWeight: 600 }}>Prometheus</span>
          </div>
          <div style={{ fontSize: 12, color: '#888', marginBottom: 12 }}>
            Scrapes benchmark metrics every 5 seconds from the controller's /metrics endpoint.
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <a href={promUrl} target="_blank" rel="noreferrer"
              style={{ padding: '6px 14px', fontSize: 12, borderRadius: 4, background: 'rgba(78,205,196,0.15)', border: '1px solid #4ecdc4', color: '#4ecdc4', textDecoration: 'none' }}>
              Open Prometheus
            </a>
            <a href={`${promUrl}/targets`} target="_blank" rel="noreferrer"
              style={{ padding: '6px 14px', fontSize: 12, borderRadius: 4, background: 'rgba(255,255,255,0.05)', border: '1px solid #555', color: '#aaa', textDecoration: 'none' }}>
              Targets
            </a>
          </div>
        </div>

        <div style={{
          background: 'var(--bg-card)', border: '1px solid var(--border)',
          borderRadius: 8, padding: 20,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
            <span style={statusDot(grafanaStatus)} />
            <span style={{ fontSize: 15, fontWeight: 600 }}>Grafana</span>
          </div>
          <div style={{ fontSize: 12, color: '#888', marginBottom: 12 }}>
            Pre-built dashboard with throughput, latency, errors, retries, CPU, memory — color-coded per language.
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <a href={`${grafUrl}/d/crdb-benchmarks`} target="_blank" rel="noreferrer"
              style={{ padding: '6px 14px', fontSize: 12, borderRadius: 4, background: 'rgba(78,205,196,0.15)', border: '1px solid #4ecdc4', color: '#4ecdc4', textDecoration: 'none' }}>
              Open Dashboard
            </a>
            <a href={grafUrl} target="_blank" rel="noreferrer"
              style={{ padding: '6px 14px', fontSize: 12, borderRadius: 4, background: 'rgba(255,255,255,0.05)', border: '1px solid #555', color: '#aaa', textDecoration: 'none' }}>
              Grafana Home
            </a>
          </div>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 8, marginBottom: 24 }}>
        <button
          onClick={async () => {
            setLaunching(true);
            try { await startMonitoring(); setTimeout(checkStatus, 3000); }
            catch {} finally { setLaunching(false); }
          }}
          disabled={launching}
          style={{
            padding: '8px 20px', fontSize: 12, borderRadius: 4, cursor: 'pointer',
            background: 'rgba(78,205,196,0.15)', border: '1px solid #4ecdc4', color: '#4ecdc4',
            fontWeight: 600, opacity: launching ? 0.5 : 1,
          }}
        >
          {launching ? 'Launching...' : 'Launch Prometheus & Grafana'}
        </button>
        <button
          onClick={async () => {
            try { await stopMonitoring(); setPrometheusStatus('down'); setGrafanaStatus('down'); }
            catch {}
          }}
          style={{
            padding: '8px 20px', fontSize: 12, borderRadius: 4, cursor: 'pointer',
            background: 'rgba(255,107,107,0.1)', border: '1px solid rgba(255,107,107,0.3)', color: '#ff6b6b',
            fontWeight: 600,
          }}
        >
          Shutdown
        </button>
        <button onClick={checkStatus} style={{
          padding: '8px 16px', fontSize: 12, borderRadius: 4, cursor: 'pointer',
          background: 'rgba(255,255,255,0.05)', border: '1px solid #555', color: '#aaa',
        }}>
          Check Status
        </button>
      </div>

      <div style={{ fontSize: 15, fontWeight: 600, marginBottom: 12 }}>Quick Start</div>
      <div style={{
        background: '#111', border: '1px solid var(--border)', borderRadius: 8,
        padding: 16, fontFamily: 'monospace', fontSize: 12, lineHeight: 2,
        color: '#c8d6e5',
      }}>
        <div style={{ color: '#888' }}># Start Prometheus + Grafana</div>
        <div>make monitoring-up</div>
        <div style={{ height: 8 }} />
        <div style={{ color: '#888' }}># Open Grafana (auto-provisioned dashboard)</div>
        <div>open {grafUrl}/d/crdb-benchmarks</div>
        <div style={{ color: '#666' }}>  Login: admin / benchmark</div>
        <div style={{ height: 8 }} />
        <div style={{ color: '#888' }}># Run a benchmark — metrics flow automatically</div>
        <div>make bench-all</div>
        <div style={{ height: 8 }} />
        <div style={{ color: '#888' }}># Stop monitoring</div>
        <div>make monitoring-down</div>
      </div>

      <div style={{ fontSize: 15, fontWeight: 600, marginTop: 24, marginBottom: 12 }}>Metrics Exported</div>
      <div style={{ overflowX: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border)' }}>
              <th style={{ textAlign: 'left', padding: '8px 12px', color: '#888' }}>Metric</th>
              <th style={{ textAlign: 'left', padding: '8px 12px', color: '#888' }}>Type</th>
              <th style={{ textAlign: 'left', padding: '8px 12px', color: '#888' }}>Description</th>
            </tr>
          </thead>
          <tbody>
            {[
              ['crdb_benchmark_ops_per_sec', 'gauge', 'Operations per second'],
              ['crdb_benchmark_latency_p50_us', 'gauge', 'P50 latency (microseconds)'],
              ['crdb_benchmark_latency_p95_us', 'gauge', 'P95 latency (microseconds)'],
              ['crdb_benchmark_latency_p99_us', 'gauge', 'P99 latency (microseconds)'],
              ['crdb_benchmark_errors_total', 'counter', 'Total errors'],
              ['crdb_benchmark_retries_total', 'counter', 'Total retries'],
              ['crdb_benchmark_ops_total', 'counter', 'Total operations'],
              ['crdb_benchmark_cpu_percent', 'gauge', 'Agent CPU usage (%)'],
              ['crdb_benchmark_mem_rss_mb', 'gauge', 'Agent memory (MB)'],
              ['crdb_benchmark_active_connections', 'gauge', 'Active DB connections'],
              ['crdb_benchmark_running', 'gauge', '1 if benchmark active'],
            ].map(([name, type, desc]) => (
              <tr key={name} style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '6px 12px', fontFamily: 'monospace', color: '#4ecdc4' }}>{name}</td>
                <td style={{ padding: '6px 12px', color: '#888' }}>{type}</td>
                <td style={{ padding: '6px 12px' }}>{desc}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div style={{ fontSize: 12, color: '#555', marginTop: 16 }}>
        All metrics labeled with <code style={{ color: '#4ecdc4' }}>language</code>, <code style={{ color: '#4ecdc4' }}>mode</code>, <code style={{ color: '#4ecdc4' }}>workload</code> dimensions.
        Scrape target: <code style={{ color: '#888' }}>http://localhost:9495/metrics</code>
      </div>
    </div>
  );
}
