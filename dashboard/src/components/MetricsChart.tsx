import { useEffect, useRef, useCallback } from 'react'
import {
  AGENTS,
  LANGUAGE_COLORS,
  type MetricSnapshot,
  type MetricKey,
  type WorkloadType,
  type ChaosEvent,
  agentKey,
} from '../types'

interface Props {
  metrics: Map<string, MetricSnapshot[]>;
  metricKey: MetricKey;
  workload: WorkloadType;
  chaosEvents: ChaosEvent[];
  hiddenAgents?: Set<string>;
  targetLine?: number;
  height?: number;
}

interface AgentSeries {
  key: string;
  label: string;
  color: string;
  dashed: boolean;
  times: number[];
  values: number[];
}

function extractPerAgent(
  metrics: Map<string, MetricSnapshot[]>,
  metricKey: MetricKey,
  workload: WorkloadType
): AgentSeries[] {
  const result: AgentSeries[] = [];
  for (const agent of AGENTS) {
    const key = agentKey(agent.language, agent.mode);
    const snaps = (metrics.get(key) ?? []).filter(s => s.workload === workload && s.ops_per_sec > 0);
    if (snaps.length === 0) continue;
    const times: number[] = [];
    const values: number[] = [];
    let t0 = 0;
    for (const s of snaps) {
      if (t0 === 0) t0 = s.ts;
      times.push((s.ts - t0) / 1000);
      values.push(s[metricKey] as number);
    }
    result.push({
      key: agentKey(agent.language, agent.mode),
      label: agent.label,
      color: agent.color || LANGUAGE_COLORS[agent.language] || '#888',
      dashed: false,
      times,
      values,
    });
  }
  return result;
}

const SCENARIO_LABELS: Record<string, string> = {
  node_kill: 'Node Kill',
  network_partition: 'Net Partition',
  region_failure: 'Region Fail',
  latency_injection: 'Latency Inj',
  rolling_restart: 'Rolling Restart',
  disk_pressure: 'Disk Pressure',
};

function drawChart(
  canvas: HTMLCanvasElement,
  series: AgentSeries[],
  height: number,
  targetLine?: number,
  chaosEvents?: ChaosEvent[],
  chartT0?: number,
) {
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.parentElement?.clientWidth ?? 600;
  canvas.width = w * dpr;
  canvas.height = height * dpr;
  canvas.style.width = `${w}px`;
  canvas.style.height = `${height}px`;

  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  ctx.scale(dpr, dpr);

  const pad = { top: 20, right: 20, bottom: 40, left: 70 };
  const plotW = w - pad.left - pad.right;
  const plotH = height - pad.top - pad.bottom;

  ctx.fillStyle = 'transparent';
  ctx.clearRect(0, 0, w, height);

  let maxT = 0;
  let maxV = 0;
  for (const s of series) {
    for (const t of s.times) if (t > maxT) maxT = t;
    for (const v of s.values) if (v > maxV) maxV = v;
  }
  if (targetLine && targetLine > maxV) maxV = targetLine;
  if (maxT === 0) maxT = 30;
  if (maxV === 0) maxV = 100;
  maxV *= 1.1;

  ctx.strokeStyle = '#333';
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(pad.left, pad.top);
  ctx.lineTo(pad.left, pad.top + plotH);
  ctx.lineTo(pad.left + plotW, pad.top + plotH);
  ctx.stroke();

  ctx.fillStyle = '#888';
  ctx.font = '11px monospace';
  ctx.textAlign = 'center';
  const xTicks = 6;
  for (let i = 0; i <= xTicks; i++) {
    const t = (maxT / xTicks) * i;
    const x = pad.left + (t / maxT) * plotW;
    ctx.fillText(`${Math.round(t)}s`, x, pad.top + plotH + 20);
    ctx.strokeStyle = '#1a1a1a';
    ctx.beginPath();
    ctx.moveTo(x, pad.top);
    ctx.lineTo(x, pad.top + plotH);
    ctx.stroke();
  }

  ctx.textAlign = 'right';
  const yTicks = 5;
  for (let i = 0; i <= yTicks; i++) {
    const v = (maxV / yTicks) * i;
    const y = pad.top + plotH - (v / maxV) * plotH;
    let label: string;
    if (maxV >= 10000) label = `${(v / 1000).toFixed(0)}k`;
    else if (maxV >= 1000) label = `${(v / 1000).toFixed(1)}k`;
    else label = v.toFixed(0);
    ctx.fillStyle = '#888';
    ctx.fillText(label, pad.left - 8, y + 4);
    ctx.strokeStyle = '#1a1a1a';
    ctx.beginPath();
    ctx.moveTo(pad.left, y);
    ctx.lineTo(pad.left + plotW, y);
    ctx.stroke();
  }

  if (targetLine && targetLine > 0) {
    const tY = pad.top + plotH - (targetLine / maxV) * plotH;
    ctx.strokeStyle = '#ffd93d';
    ctx.lineWidth = 1.5;
    ctx.setLineDash([8, 4]);
    ctx.beginPath();
    ctx.moveTo(pad.left, tY);
    ctx.lineTo(pad.left + plotW, tY);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = '#ffd93d';
    ctx.font = '10px sans-serif';
    ctx.textAlign = 'left';
    ctx.fillText(`Target: ${targetLine >= 1000 ? (targetLine/1000).toFixed(0)+'k' : targetLine} ops/s`, pad.left + 4, tY - 4);
  }

  for (const s of series) {
    if (s.times.length < 2) continue;
    ctx.strokeStyle = s.color;
    ctx.lineWidth = 2;
    if (s.dashed) ctx.setLineDash([6, 4]);
    else ctx.setLineDash([]);

    ctx.beginPath();
    for (let i = 0; i < s.times.length; i++) {
      const x = pad.left + (s.times[i] / maxT) * plotW;
      const y = pad.top + plotH - (s.values[i] / maxV) * plotH;
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    }
    ctx.stroke();

    for (let i = 0; i < s.times.length; i++) {
      const x = pad.left + (s.times[i] / maxT) * plotW;
      const y = pad.top + plotH - (s.values[i] / maxV) * plotH;
      ctx.fillStyle = s.color;
      ctx.beginPath();
      ctx.arc(x, y, 2.5, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  ctx.setLineDash([]);
  const legendY = pad.top + 4;
  let legendX = pad.left + 8;
  ctx.font = '11px sans-serif';
  for (const s of series) {
    ctx.strokeStyle = s.color;
    ctx.lineWidth = 2;
    if (s.dashed) ctx.setLineDash([4, 3]);
    else ctx.setLineDash([]);
    ctx.beginPath();
    ctx.moveTo(legendX, legendY);
    ctx.lineTo(legendX + 20, legendY);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = '#ccc';
    ctx.textAlign = 'left';
    ctx.fillText(s.label, legendX + 24, legendY + 4);
    legendX += ctx.measureText(s.label).width + 40;
  }

  if (chaosEvents && chaosEvents.length > 0 && chartT0 && maxT > 0) {
    for (const ev of chaosEvents) {
      const evTime = new Date(ev.timestamp).getTime();
      const evSec = (evTime - chartT0) / 1000;
      if (evSec < 0 || evSec > maxT) continue;

      const x = pad.left + (evSec / maxT) * plotW;

      ctx.strokeStyle = '#ff4444';
      ctx.lineWidth = 1.5;
      ctx.setLineDash([6, 4]);
      ctx.beginPath();
      ctx.moveTo(x, pad.top);
      ctx.lineTo(x, pad.top + plotH);
      ctx.stroke();
      ctx.setLineDash([]);

      const label = SCENARIO_LABELS[ev.scenario] || ev.scenario;
      ctx.save();
      ctx.font = '9px sans-serif';
      ctx.fillStyle = '#ff4444';
      ctx.textAlign = 'center';
      ctx.fillText(label, x, pad.top - 4);
      ctx.restore();
    }
  }
}

export function MetricsChart({ metrics, metricKey, workload, chaosEvents, hiddenAgents, targetLine, height = 340 }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const lastWidthRef = useRef(0);
  const rafRef = useRef(0);

  const render = useCallback(() => {
    if (!canvasRef.current || !containerRef.current) return;
    const w = containerRef.current.clientWidth;
    if (w <= 0) return;
    lastWidthRef.current = w;
    const allSeries = extractPerAgent(metrics, metricKey, workload);
    const series = hiddenAgents?.size
      ? allSeries.filter(s => !hiddenAgents.has(s.key))
      : allSeries;

    let chartT0: number | undefined;
    for (const snaps of metrics.values()) {
      if (snaps.length > 0) {
        const first = snaps[0].ts;
        if (!chartT0 || first < chartT0) chartT0 = first;
      }
    }

    drawChart(
      canvasRef.current,
      series,
      height,
      metricKey === 'ops_per_sec' ? targetLine : undefined,
      chaosEvents,
      chartT0,
    );
  }, [metrics, metricKey, workload, height, hiddenAgents, targetLine, chaosEvents]);

  useEffect(() => {
    render();
  }, [render]);

  useEffect(() => {
    if (!containerRef.current) return;
    const observer = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect?.width ?? 0;
      if (w > 0 && Math.abs(w - lastWidthRef.current) > 2) {
        cancelAnimationFrame(rafRef.current);
        rafRef.current = requestAnimationFrame(() => render());
      }
    });
    observer.observe(containerRef.current);
    return () => { observer.disconnect(); cancelAnimationFrame(rafRef.current); };
  }, [render]);

  return (
    <div
      ref={containerRef}
      style={{
        background: 'var(--bg-card)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--radius-lg)',
        padding: 12,
        overflow: 'hidden',
        minHeight: height,
      }}
    >
      <canvas ref={canvasRef} />
    </div>
  )
}
