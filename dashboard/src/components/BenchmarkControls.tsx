import { useState, useEffect, useRef } from 'react'
import {
  WORKLOAD_TYPES,
  WORKLOAD_LABELS,
  type WorkloadType,
  type Topology,
  type ChaosMode,
} from '../types'

type ChaosScenario = 'node_kill' | 'network_partition' | 'region_failure' | 'latency_injection' | 'rolling_restart' | 'disk_pressure';

const CHAOS_SCENARIOS: { value: ChaosScenario; label: string; icon: string }[] = [
  { value: 'node_kill', label: 'Node Kill', icon: '💀' },
  { value: 'network_partition', label: 'Net Partition', icon: '🔌' },
  { value: 'region_failure', label: 'Region Failure', icon: '🌍' },
  { value: 'latency_injection', label: 'Latency', icon: '🐌' },
  { value: 'rolling_restart', label: 'Rolling Restart', icon: '🔄' },
  { value: 'disk_pressure', label: 'Disk I/O', icon: '💾' },
];
import { useApi, type ScenarioListItem } from '../hooks/useApi'

interface Props {
  onStarted?: (runId: string) => void;
  onStopped?: () => void;
  onTargetChange?: (target: number) => void;
  hideWorkloads?: boolean;
  hideScenarios?: boolean;
}

const styles = {
  container: {
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--radius-lg)',
    padding: 16,
    marginBottom: 16,
  } as const,
  row: {
    display: 'flex',
    gap: 12,
    alignItems: 'center',
    flexWrap: 'wrap',
    marginBottom: 12,
  } as const,
  label: {
    fontSize: 12,
    color: 'var(--text-secondary)',
    fontWeight: 500,
    minWidth: 60,
  } as const,
  checkboxGroup: {
    display: 'flex',
    gap: 10,
    flexWrap: 'wrap',
  } as const,
  checkbox: {
    display: 'flex',
    alignItems: 'center',
    gap: 4,
    fontSize: 12,
    color: 'var(--text-primary)',
    cursor: 'pointer',
  } as const,
  actions: {
    display: 'flex',
    gap: 8,
    marginTop: 4,
  } as const,
  btnStart: {
    padding: '8px 20px',
    background: 'var(--accent)',
    color: '#0a0a0a',
    borderRadius: 'var(--radius)',
    fontWeight: 600,
    fontSize: 13,
  } as const,
  btnStop: {
    padding: '8px 20px',
    background: 'var(--danger)',
    color: '#fff',
    borderRadius: 'var(--radius)',
    fontWeight: 600,
    fontSize: 13,
  } as const,
  error: {
    color: 'var(--danger)',
    fontSize: 12,
    marginTop: 8,
  } as const,
}

export function BenchmarkControls({ onStarted, onStopped, onTargetChange, hideWorkloads, hideScenarios }: Props) {
  const { startBenchmark, stopBenchmark, getStatus, teardown, getScenarios, runScenario, uploadScenario } = useApi();
  const pollRef = useRef<ReturnType<typeof setInterval>>(undefined);
  const [topology, setTopology] = useState<Topology>('single');
  const [workloads, setWorkloads] = useState<Set<WorkloadType>>(new Set(['crud']));
  const [chaosMode, setChaosMode] = useState<ChaosMode | ''>('');
  const [chaosScenarios, setChaosScenarios] = useState<Set<ChaosScenario>>(
    new Set(CHAOS_SCENARIOS.map(s => s.value))
  );
  const [duration, setDuration] = useState(60);
  const [warmup, setWarmup] = useState(10);
  const [iterations, setIterations] = useState(1);
  const [concurrency, setConcurrency] = useState(8);
  const [targetOps, setTargetOps] = useState(0);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');
  const [languages, setLanguages] = useState<Set<string>>(new Set(['java', 'go', 'python', 'dotnet', 'java-native']));
  const [isolation, setIsolation] = useState(false);
  const [startTime, setStartTime] = useState<number | null>(null);
  const [elapsed, setElapsed] = useState(0);
  const [finalTime, setFinalTime] = useState<number | null>(null);
  const [blinking, setBlinking] = useState(false);
  const [scenarios, setScenarios] = useState<ScenarioListItem[]>([]);
  const [selectedScenario, setSelectedScenario] = useState('');
  const [customJSON, setCustomJSON] = useState('');
  const [showCustomEditor, setShowCustomEditor] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const timerRef = useRef<ReturnType<typeof setInterval>>(undefined);
  const runningRef = useRef(false);
  const startTimeRef = useRef<number | null>(null);

  useEffect(() => {
    getScenarios().then(setScenarios).catch(() => {});
  }, [getScenarios]);

  useEffect(() => {
    if (running && startTime !== null) {
      startTimeRef.current = startTime;
      timerRef.current = setInterval(() => {
        if (startTimeRef.current) {
          setElapsed(Math.floor((Date.now() - startTimeRef.current) / 1000));
        }
      }, 1000);
      return () => clearInterval(timerRef.current);
    }
    return () => clearInterval(timerRef.current);
  }, [running, startTime]);

  useEffect(() => {
    runningRef.current = running;
  }, [running]);

  useEffect(() => {
    getStatus().then(s => {
      if (s.benchmark) {
        setRunning(true);
        if (!startTimeRef.current) {
          setStartTime(Date.now());
          setFinalTime(null);
        }
      }
    }).catch(() => {});
    pollRef.current = setInterval(() => {
      getStatus().then(s => {
        if (runningRef.current && !s.benchmark) {
          setFinalTime(Math.floor((Date.now() - (startTimeRef.current || Date.now())) / 1000));
          setRunning(false);
          setStartTime(null);
          setBlinking(true);
          setTimeout(() => setBlinking(false), 5000);
        }
        if (!runningRef.current && s.benchmark) {
          setRunning(true);
          if (!startTimeRef.current) setStartTime(Date.now());
        }
      }).catch(() => {});
    }, 3000);
    return () => clearInterval(pollRef.current);
  }, [getStatus]);

  const formatElapsed = (secs: number) => {
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  const toggleLanguage = (lang: string) => {
    setLanguages(prev => {
      const next = new Set(prev);
      if (next.has(lang)) next.delete(lang);
      else next.add(lang);
      return next;
    });
  };

  const toggleWorkload = (w: WorkloadType) => {
    setWorkloads((prev) => {
      const next = new Set(prev);
      if (next.has(w)) next.delete(w);
      else next.add(w);
      return next;
    });
  };

  const toggleChaos = (s: ChaosScenario) => {
    setChaosScenarios(prev => {
      const next = new Set(prev);
      if (next.has(s)) next.delete(s);
      else next.add(s);
      return next;
    });
  };

  const handleScenarioStart = async () => {
    if (!selectedScenario && !customJSON) return;
    setError('');
    try {
      const res = await runScenario(selectedScenario);
      setRunning(true);
      setStartTime(Date.now());
      setElapsed(0);
      setFinalTime(null);
      onStarted?.(res.run_id);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setError('');
    try {
      await uploadScenario(file);
      const updated = await getScenarios();
      setScenarios(updated);
    } catch (err) {
      setError((err as Error).message);
    }
    if (fileInputRef.current) fileInputRef.current.value = '';
  };

  const handleStart = async () => {
    setError('');
    try {
      const res = await startBenchmark({
        workloads: Array.from(workloads),
        topology,
        duration_secs: duration,
        warmup_secs: warmup,
        iterations,
        concurrency,
        languages: Array.from(languages),
        isolation,
        ...(chaosMode ? {
          chaos_mode: chaosMode,
          chaos_scenarios: Array.from(chaosScenarios),
        } : {}),
      } as any);
      setRunning(true);
      setStartTime(Date.now());
      setElapsed(0);
      setFinalTime(null);
      onStarted?.(res.run_id);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const handleStop = async () => {
    setError('');
    try {
      await stopBenchmark();
      setFinalTime(elapsed);
      setRunning(false);
      setStartTime(null);
      onStopped?.();
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <div style={styles.container}>
      <div style={styles.row}>
        <span style={styles.label}>Topology</span>
        <select value={topology} onChange={(e) => setTopology(e.target.value as Topology)}>
          <option value="single">Single</option>
          <option value="cluster">Cluster</option>
          <option value="multi">Multi-Region</option>
        </select>

        <span style={{ ...styles.label, marginLeft: 8 }}>Duration</span>
        <input
          type="number"
          value={duration}
          onChange={(e) => setDuration(Number(e.target.value))}
          style={{ width: 70 }}
          min={10}
          max={3600}
        />
        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>sec</span>

        <span style={{ ...styles.label, marginLeft: 8 }}>Warmup</span>
        <input
          type="number"
          value={warmup}
          onChange={(e) => setWarmup(Number(e.target.value))}
          style={{ width: 60 }}
          min={0}
          max={60}
        />
        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>sec</span>

        <span style={{ ...styles.label, marginLeft: 8 }}>Iterations</span>
        <input
          type="number"
          value={iterations}
          onChange={(e) => setIterations(Math.max(1, Math.min(10, Number(e.target.value))))}
          style={{ width: 60 }}
          min={1}
          max={10}
        />

        <span style={{ ...styles.label, marginLeft: 8 }}>Concurrency</span>
        <input
          type="number"
          value={concurrency}
          onChange={(e) => setConcurrency(Number(e.target.value))}
          style={{ width: 60 }}
          min={1}
          max={512}
        />

        <span style={{ ...styles.label, marginLeft: 8 }}>Target</span>
        <input
          type="number"
          value={targetOps || ''}
          onChange={(e) => { const v = Number(e.target.value); setTargetOps(v); onTargetChange?.(v); }}
          style={{ width: 80 }}
          min={0}
          placeholder="ops/s"
        />
        <span style={{ fontSize: 10, color: '#666' }}>ops/s goal</span>

        <span style={{ ...styles.label, marginLeft: 8 }}>Chaos</span>
        <select
          value={chaosMode}
          onChange={(e) => setChaosMode(e.target.value as ChaosMode | '')}
          disabled={topology !== 'multi'}
          style={{ opacity: topology !== 'multi' ? 0.4 : 1 }}
        >
          <option value="">None</option>
          <option value="individual">Individual</option>
          <option value="gauntlet">Gauntlet (all 6)</option>
          <option value="random">Random</option>
        </select>
        {topology !== 'multi' && chaosMode === '' && (
          <span style={{ fontSize: 10, color: '#666' }}>requires Multi-Region</span>
        )}
      </div>

      {chaosMode && (
        <div style={{
          ...styles.row,
          background: topology === 'multi'
            ? 'rgba(255,107,107,0.05)'
            : 'rgba(255,255,255,0.02)',
          border: `1px solid ${topology === 'multi' ? 'rgba(255,107,107,0.2)' : '#222'}`,
          borderRadius: 6,
          padding: '8px 12px',
          marginBottom: 12,
          opacity: topology === 'multi' ? 1 : 0.5,
        }}>
          <span style={{ ...styles.label, color: '#ff6b6b' }}>Scenarios</span>
          <div style={styles.checkboxGroup}>
            {CHAOS_SCENARIOS.map(s => (
              <label key={s.value} style={{
                ...styles.checkbox,
                opacity: chaosScenarios.has(s.value) ? 1 : 0.4,
              }}>
                <input
                  type="checkbox"
                  checked={chaosScenarios.has(s.value)}
                  onChange={() => toggleChaos(s.value)}
                  disabled={topology !== 'multi'}
                />
                <span>{s.icon}</span>
                <span>{s.label}</span>
              </label>
            ))}
          </div>
        </div>
      )}

      <div style={styles.row}>
        <span style={styles.label}>Languages</span>
        <div style={styles.checkboxGroup}>
          {[
            { key: 'java', label: 'Java' },
            { key: 'java-native', label: 'Java Native (GraalVM)' },
            { key: 'go', label: 'Go' },
            { key: 'python', label: 'Python' },
            { key: 'dotnet', label: '.NET' },
          ].map(lang => (
            <label key={lang.key} style={styles.checkbox}>
              <input
                type="checkbox"
                checked={languages.has(lang.key)}
                onChange={() => toggleLanguage(lang.key)}
              />
              {lang.label}
            </label>
          ))}
        </div>
        <label style={{ ...styles.checkbox, marginLeft: 16, opacity: 0.6 }}>
          <input
            type="checkbox"
            checked={isolation}
            onChange={() => setIsolation(prev => !prev)}
          />
          <span>Isolation Mode</span>
          <span style={{ fontSize: 10, color: '#888', marginLeft: 4 }}>(coming soon)</span>
        </label>
      </div>

      {!hideWorkloads && (
        <div style={styles.row}>
          <span style={styles.label}>Workloads</span>
          <div style={styles.checkboxGroup}>
            {WORKLOAD_TYPES.map((w) => (
              <label key={w} style={styles.checkbox}>
                <input
                  type="checkbox"
                  checked={workloads.has(w)}
                  onChange={() => toggleWorkload(w)}
                />
                {WORKLOAD_LABELS[w]}
              </label>
            ))}
          </div>
        </div>
      )}

      {!hideScenarios && (<>
      <div style={{ ...styles.row, borderTop: '1px solid var(--border)', paddingTop: 12 }}>
        <span style={styles.label}>Scenarios</span>
        <select
          value={selectedScenario}
          onChange={(e) => {
            const val = e.target.value;
            setSelectedScenario(val);
            setShowCustomEditor(val === '__custom__');
          }}
          style={{ minWidth: 200 }}
        >
          <option value="">-- Select a Scenario --</option>
          <optgroup label="Built-in Workloads">
            {scenarios.filter(s => s.source === 'builtin').map(s => (
              <option key={s.name} value={s.name}>{s.name} ({s.duration_secs}s)</option>
            ))}
          </optgroup>
          <optgroup label="Custom Scenarios">
            {scenarios.filter(s => s.source !== 'builtin').map(s => (
              <option key={s.name} value={s.name}>{s.name} ({s.duration_secs}s)</option>
            ))}
          </optgroup>
          <optgroup label="Advanced">
            <option value="__custom__">Write Custom JSON...</option>
          </optgroup>
        </select>
        <button
          style={{
            padding: '6px 14px',
            background: 'var(--accent)',
            color: '#0a0a0a',
            borderRadius: 'var(--radius)',
            fontWeight: 600,
            fontSize: 12,
            cursor: running || !selectedScenario || selectedScenario === '__custom__' ? 'not-allowed' : 'pointer',
            opacity: running || !selectedScenario || selectedScenario === '__custom__' ? 0.4 : 1,
          }}
          onClick={handleScenarioStart}
          disabled={running || !selectedScenario || selectedScenario === '__custom__'}
        >
          Run Scenario
        </button>
        <input
          ref={fileInputRef}
          type="file"
          accept=".json"
          style={{ display: 'none' }}
          onChange={handleUpload}
        />
        <button
          onClick={() => fileInputRef.current?.click()}
          style={{
            padding: '6px 14px',
            background: 'rgba(78,205,196,0.1)',
            color: '#4ecdc4',
            borderRadius: 'var(--radius)',
            fontWeight: 600,
            fontSize: 12,
            border: '1px solid rgba(78,205,196,0.3)',
            cursor: 'pointer',
          }}
        >
          Upload .json
        </button>
      </div>

      {selectedScenario && selectedScenario !== '__custom__' && (() => {
        const s = scenarios.find(sc => sc.name === selectedScenario);
        if (!s) return null;
        return (
          <div style={{
            padding: 12, marginBottom: 12,
            background: 'rgba(78,205,196,0.03)', border: '1px solid rgba(78,205,196,0.15)',
            borderRadius: 6, fontSize: 12, lineHeight: 1.6,
          }}>
            <div style={{ fontWeight: 600, color: '#4ecdc4', marginBottom: 4 }}>{s.name}</div>
            <div style={{ color: '#aaa' }}>{s.description}</div>
            <div style={{ display: 'flex', gap: 16, marginTop: 8, color: '#888', fontSize: 11 }}>
              <span>Duration: {s.duration_secs}s</span>
              <span>Operations: {s.operations}</span>
              <span>Source: {s.source}</span>
            </div>
          </div>
        );
      })()}

      {showCustomEditor && (
        <div style={{ marginBottom: 12 }}>
          <textarea
            value={customJSON}
            onChange={(e) => setCustomJSON(e.target.value)}
            placeholder='{"name": "my-scenario", "description": "...", "duration_secs": 30, "operations": [...]}'
            style={{
              width: '100%',
              minHeight: 120,
              fontFamily: 'monospace',
              fontSize: 12,
              background: 'var(--bg-card)',
              color: 'var(--text-primary)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius)',
              padding: 8,
              resize: 'vertical',
            }}
          />
        </div>
      )}
      </>)}

      <div style={styles.actions}>
        <button
          style={{
            ...styles.btnStart,
            opacity: running ? 0.4 : 1,
            cursor: running ? 'not-allowed' : 'pointer',
          }}
          onClick={handleStart}
          disabled={workloads.size === 0 || running}
        >
          {running ? 'Running...' : 'Start Benchmark'}
        </button>
        <button
          style={{
            ...styles.btnStop,
            opacity: running ? 1 : 0.3,
            cursor: running ? 'pointer' : 'not-allowed',
          }}
          onClick={handleStop}
          disabled={!running}
        >
          Abort
        </button>
        <button
          onClick={async () => {
            setError('');
            try {
              await teardown();
              setRunning(false);
            } catch (e) {
              setError((e as Error).message);
            }
          }}
          style={{
            padding: '8px 16px',
            background: 'rgba(255,107,107,0.1)',
            color: '#ff6b6b',
            borderRadius: 'var(--radius)',
            fontWeight: 600,
            fontSize: 12,
            border: '1px solid rgba(255,107,107,0.3)',
            cursor: 'pointer',
            marginLeft: 8,
          }}
        >
          Tear Down All
        </button>
        {running && (
          <span style={{ fontSize: 11, color: '#4ecdc4', display: 'flex', alignItems: 'center', gap: 4, marginLeft: 8 }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: '#4ecdc4', animation: 'pulse 1.5s infinite' }} />
            Benchmark in progress
            <span style={{ fontFamily: 'monospace', fontWeight: 600, marginLeft: 4 }}>
              {formatElapsed(elapsed)}
            </span>
          </span>
        )}
        {!running && finalTime !== null && (
          <span style={{
            fontSize: 11,
            color: blinking ? '#4ecdc4' : '#888',
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            marginLeft: 8,
            animation: blinking ? 'blink 0.8s ease-in-out 6' : 'none',
          }}>
            <span style={{ fontSize: 14 }}>{blinking ? '✓' : ''}</span>
            Completed in {formatElapsed(finalTime)}
          </span>
        )}
      </div>

      {error && <div style={styles.error}>{error}</div>}
    </div>
  )
}
