import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useApi, type ScenarioListItem } from '../hooks/useApi'

const SCENARIO_ICONS: Record<string, string> = {
  'Banking Transfer': '🏦',
  'E-Commerce Checkout': '🛒',
  'IoT Sensor Ingestion': '📡',
  'Multi-Tenant SaaS': '🏢',
  'Gaming & Sports Betting': '🎮',
  'Inventory Management': '📦',
};

export function ScenariosPage() {
  const { getScenarios } = useApi();
  const navigate = useNavigate();
  const [scenarios, setScenarios] = useState<ScenarioListItem[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getScenarios()
      .then(s => setScenarios(s ?? []))
      .catch(() => setScenarios([]))
      .finally(() => setLoading(false));
  }, [getScenarios]);

  const builtins = scenarios.filter(s => s.source === 'builtin');
  const realWorld = scenarios.filter(s => s.source === 'file');
  const custom = scenarios.filter(s => s.source === 'custom');

  const handleSelect = (scenarioName: string) => {
    navigate('/live', { state: { scenario: scenarioName } });
  };

  const handleStandardSuite = () => {
    navigate('/live', { state: { scenario: '__standard_suite__' } });
  };

  if (loading) {
    return <div style={{ padding: 40, textAlign: 'center', color: '#888' }}>Loading scenarios...</div>;
  }

  return (
    <div style={{ paddingBottom: 220 }}>
      <div style={{ fontSize: 20, fontWeight: 600, marginBottom: 4 }}>Benchmark Scenarios</div>
      <div style={{ fontSize: 13, color: '#888', marginBottom: 24 }}>
        Select a scenario to configure and run. Each scenario defines SQL operations, concurrency, and assertions for a specific use case.
      </div>

      <div style={{
        fontSize: 14, fontWeight: 600, color: '#aaa', marginBottom: 12,
        borderBottom: '1px solid var(--border)', paddingBottom: 6,
      }}>
        Standard Benchmark Suite
      </div>

      <div
        onClick={handleStandardSuite}
        style={{
          background: 'var(--bg-card)', border: '1px solid var(--border)',
          borderRadius: 8, padding: 20, cursor: 'pointer', marginBottom: 28,
          transition: 'all 0.2s',
        }}
        onMouseEnter={e => { (e.currentTarget as HTMLElement).style.borderColor = '#4ecdc4'; (e.currentTarget as HTMLElement).style.background = '#161616'; }}
        onMouseLeave={e => { (e.currentTarget as HTMLElement).style.borderColor = 'var(--border)'; (e.currentTarget as HTMLElement).style.background = 'var(--bg-card)'; }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
          <span style={{ fontSize: 24 }}>🔬</span>
          <span style={{ fontSize: 17, fontWeight: 600, color: 'var(--text-primary)' }}>Standard CockroachDB Library Benchmark</span>
        </div>
        <div style={{ fontSize: 12, color: '#999', lineHeight: 1.6, marginBottom: 12 }}>
          Runs all 6 core workloads sequentially to measure library overhead, retry handling, read routing, CDC throughput, and vector search performance.
          Compares your CockroachDB client libraries against vanilla driver baselines across every operation type.
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 10 }}>
          {builtins.map(s => (
            <span key={s.name} style={{
              padding: '3px 10px', fontSize: 11, borderRadius: 4,
              background: 'rgba(78,205,196,0.1)', border: '1px solid rgba(78,205,196,0.2)', color: '#4ecdc4',
            }}>
              {s.name}
            </span>
          ))}
        </div>
        <div style={{ fontSize: 11, color: '#666' }}>
          {builtins.length} workloads • ~{builtins.reduce((sum, s) => sum + s.duration_secs, 0)}s total • {builtins.reduce((sum, s) => sum + s.operations, 0)} operations
        </div>
      </div>

      {realWorld.length > 0 && (
        <>
          <div style={{
            fontSize: 14, fontWeight: 600, color: '#aaa', marginBottom: 12,
            borderBottom: '1px solid var(--border)', paddingBottom: 6,
          }}>
            Real-World Scenarios
          </div>
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))',
            gap: 12, marginBottom: 28,
          }}>
            {realWorld.map(s => (
              <div
                key={s.name}
                onClick={() => handleSelect(s.name)}
                style={{
                  background: 'var(--bg-card)', border: '1px solid var(--border)',
                  borderRadius: 8, padding: 16, cursor: 'pointer', transition: 'all 0.2s',
                }}
                onMouseEnter={e => { (e.currentTarget as HTMLElement).style.borderColor = '#4ecdc4'; (e.currentTarget as HTMLElement).style.background = '#161616'; }}
                onMouseLeave={e => { (e.currentTarget as HTMLElement).style.borderColor = 'var(--border)'; (e.currentTarget as HTMLElement).style.background = 'var(--bg-card)'; }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                  <span style={{ fontSize: 20 }}>{SCENARIO_ICONS[s.name] || '📋'}</span>
                  <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)' }}>{s.name}</span>
                </div>
                <div style={{
                  fontSize: 12, color: '#888', lineHeight: 1.5, marginBottom: 10,
                  display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden',
                }}>
                  {s.description}
                </div>
                <div style={{ display: 'flex', gap: 12, fontSize: 11, color: '#666' }}>
                  <span style={{
                    padding: '2px 8px', background: 'rgba(78,205,196,0.1)',
                    border: '1px solid rgba(78,205,196,0.2)', borderRadius: 4, color: '#4ecdc4',
                  }}>
                    {s.operations} ops
                  </span>
                  <span style={{
                    padding: '2px 8px', background: 'rgba(255,255,255,0.03)',
                    border: '1px solid #333', borderRadius: 4,
                  }}>
                    {s.duration_secs}s
                  </span>
                </div>
              </div>
            ))}
          </div>
        </>
      )}

      {custom.length > 0 && (
        <>
          <div style={{
            fontSize: 14, fontWeight: 600, color: '#aaa', marginBottom: 12,
            borderBottom: '1px solid var(--border)', paddingBottom: 6,
          }}>
            Custom Scenarios
          </div>
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))',
            gap: 12, marginBottom: 28,
          }}>
            {custom.map(s => (
              <div
                key={s.name}
                onClick={() => handleSelect(s.name)}
                style={{
                  background: 'var(--bg-card)', border: '1px solid var(--border)',
                  borderRadius: 8, padding: 16, cursor: 'pointer', transition: 'all 0.2s',
                }}
                onMouseEnter={e => { (e.currentTarget as HTMLElement).style.borderColor = '#c084fc'; (e.currentTarget as HTMLElement).style.background = '#161616'; }}
                onMouseLeave={e => { (e.currentTarget as HTMLElement).style.borderColor = 'var(--border)'; (e.currentTarget as HTMLElement).style.background = 'var(--bg-card)'; }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                  <span style={{ fontSize: 20 }}>🔧</span>
                  <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)' }}>{s.name}</span>
                </div>
                <div style={{ fontSize: 12, color: '#888', lineHeight: 1.5, marginBottom: 10 }}>
                  {s.description}
                </div>
                <div style={{ display: 'flex', gap: 12, fontSize: 11, color: '#666' }}>
                  <span style={{ padding: '2px 8px', background: 'rgba(192,132,252,0.1)', border: '1px solid rgba(192,132,252,0.2)', borderRadius: 4, color: '#c084fc' }}>
                    {s.operations} ops
                  </span>
                  <span style={{ padding: '2px 8px', background: 'rgba(255,255,255,0.03)', border: '1px solid #333', borderRadius: 4 }}>
                    {s.duration_secs}s
                  </span>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
