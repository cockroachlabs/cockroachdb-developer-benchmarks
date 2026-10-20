import { useState, useEffect, useCallback } from 'react'
import { type ChaosEvent } from '../types'
import { useApi } from '../hooks/useApi'

interface Props {
  topology: string;
  benchmarkRunning: boolean;
  chaosEvents: ChaosEvent[];
}

const SCENARIOS = [
  { id: 'node_kill', label: 'Node Kill', icon: '💀', desc: 'Stop and restart a node' },
  { id: 'network_partition', label: 'Net Partition', icon: '🔌', desc: 'Isolate a node via iptables' },
  { id: 'region_failure', label: 'Region Failure', icon: '🌍', desc: 'Take down an entire region' },
  { id: 'latency_injection', label: 'Latency Inject', icon: '🐌', desc: 'Add 200ms network delay' },
  { id: 'rolling_restart', label: 'Rolling Restart', icon: '🔄', desc: 'Restart nodes one by one' },
  { id: 'disk_pressure', label: 'Disk Pressure', icon: '💾', desc: 'Fill disk with temp data' },
] as const;

const styles = {
  container: {
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--radius-lg)',
    padding: 16,
    marginBottom: 12,
  } as const,
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  } as const,
  title: {
    fontSize: 14,
    fontWeight: 600,
    color: 'var(--text-primary)',
  } as const,
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(3, 1fr)',
    gap: 8,
    marginBottom: 12,
  } as const,
  btn: {
    display: 'flex',
    flexDirection: 'column' as const,
    alignItems: 'center',
    gap: 4,
    padding: '10px 8px',
    fontSize: 11,
    fontWeight: 500,
    borderRadius: 6,
    cursor: 'pointer',
    border: '1px solid #333',
    background: 'rgba(255,255,255,0.03)',
    color: '#ccc',
    transition: 'all 0.2s',
  } as const,
  btnDisabled: {
    opacity: 0.4,
    cursor: 'not-allowed',
  } as const,
  btnIcon: {
    fontSize: 18,
  } as const,
  warning: {
    padding: '10px 14px',
    fontSize: 12,
    color: '#ffd93d',
    background: 'rgba(255,217,61,0.08)',
    border: '1px solid rgba(255,217,61,0.2)',
    borderRadius: 6,
    textAlign: 'center' as const,
  } as const,
  plansRow: {
    display: 'flex',
    gap: 8,
    alignItems: 'center',
    marginBottom: 12,
  } as const,
  select: {
    flex: 1,
    padding: '6px 10px',
    fontSize: 12,
    borderRadius: 4,
    border: '1px solid #333',
    background: 'var(--bg-card)',
    color: 'var(--text-primary)',
  } as const,
  runBtn: {
    padding: '6px 14px',
    fontSize: 12,
    borderRadius: 4,
    cursor: 'pointer',
    border: '1px solid #ff6b6b',
    background: 'rgba(255,107,107,0.15)',
    color: '#ff6b6b',
    fontWeight: 600,
  } as const,
  eventList: {
    maxHeight: 120,
    overflowY: 'auto' as const,
    fontSize: 11,
    color: '#888',
  } as const,
  eventItem: {
    padding: '3px 0',
    borderBottom: '1px solid rgba(255,255,255,0.04)',
    display: 'flex',
    justifyContent: 'space-between',
  } as const,
};

interface ChaosPlanInfo {
  name: string;
  description: string;
  events: unknown[];
}

export function ChaosPanel({ topology, benchmarkRunning, chaosEvents }: Props) {
  const { triggerChaos, getChaosPlans, runChaosPlan } = useApi();
  const [triggering, setTriggering] = useState<string | null>(null);
  const [plans, setPlans] = useState<ChaosPlanInfo[]>([]);
  const [selectedPlan, setSelectedPlan] = useState('');
  const [planRunning, setPlanRunning] = useState(false);

  const isMulti = topology === 'multi';

  useEffect(() => {
    getChaosPlans().then(setPlans).catch(() => {});
  }, [getChaosPlans]);

  const handleTrigger = useCallback(async (scenario: string) => {
    setTriggering(scenario);
    try {
      await triggerChaos(scenario);
    } catch {
    } finally {
      setTimeout(() => setTriggering(null), 2000);
    }
  }, [triggerChaos]);

  const handleRunPlan = useCallback(async () => {
    if (!selectedPlan) return;
    setPlanRunning(true);
    try {
      await runChaosPlan(selectedPlan);
    } catch {
    } finally {
      setTimeout(() => setPlanRunning(false), 3000);
    }
  }, [selectedPlan, runChaosPlan]);

  if (!benchmarkRunning) return null;

  const recentEvents = chaosEvents.slice(-10).reverse();

  return (
    <div style={styles.container}>
      <div style={styles.header}>
        <div style={styles.title}>Chaos Engineering</div>
        {chaosEvents.length > 0 && (
          <span style={{ fontSize: 11, color: '#888' }}>
            {chaosEvents.length} event{chaosEvents.length !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      {!isMulti ? (
        <div style={styles.warning}>
          Requires Multi-Region topology
        </div>
      ) : (
        <>
          <div style={styles.grid}>
            {SCENARIOS.map(s => {
              const isActive = triggering === s.id;
              return (
                <div
                  key={s.id}
                  onClick={() => !isActive && handleTrigger(s.id)}
                  style={{
                    ...styles.btn,
                    ...(isActive ? { borderColor: '#ff6b6b', background: 'rgba(255,107,107,0.15)', color: '#ff6b6b' } : {}),
                  }}
                  title={s.desc}
                >
                  <span style={styles.btnIcon}>{s.icon}</span>
                  <span>{s.label}</span>
                </div>
              );
            })}
          </div>

          {plans.length > 0 && (
            <div style={styles.plansRow}>
              <select
                style={styles.select}
                value={selectedPlan}
                onChange={e => setSelectedPlan(e.target.value)}
              >
                <option value="">Select a chaos plan...</option>
                {plans.map(p => (
                  <option key={p.name} value={p.name.toLowerCase().replace(/\s+/g, '-')}>
                    {p.name} — {p.description}
                  </option>
                ))}
              </select>
              <button
                style={{
                  ...styles.runBtn,
                  ...(planRunning || !selectedPlan ? styles.btnDisabled : {}),
                }}
                onClick={handleRunPlan}
                disabled={planRunning || !selectedPlan}
              >
                {planRunning ? 'Running...' : 'Run Plan'}
              </button>
            </div>
          )}
        </>
      )}

      {recentEvents.length > 0 && (
        <div style={styles.eventList}>
          {recentEvents.map((ev, i) => (
            <div key={i} style={styles.eventItem}>
              <span>
                <span style={{ color: '#ff6b6b' }}>{ev.scenario}</span>
                {' '}{ev.action} on {ev.target}
              </span>
              <span>{new Date(ev.timestamp).toLocaleTimeString()}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
