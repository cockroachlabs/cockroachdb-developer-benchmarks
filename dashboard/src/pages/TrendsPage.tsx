import { useState, useEffect, useRef, useCallback } from 'react'
import { useApi } from '../hooks/useApi'
import {
  type TrendPoint,
  type RegressionItem,
  type WorkloadType,
  WORKLOAD_TYPES,
  WORKLOAD_LABELS,
  AGENTS,
} from '../types'

const AGENT_COLORS: Record<string, string> = {}
for (const a of AGENTS) {
  AGENT_COLORS[`${a.language}/${a.mode}`] = a.color
}

interface AgentLine {
  key: string
  label: string
  color: string
  points: { runId: string; timestamp: string; avgOps: number }[]
}

function buildLines(trends: TrendPoint[], workload: string): AgentLine[] {
  const byAgent = new Map<string, AgentLine>()

  for (const t of trends) {
    if (workload !== 'all' && t.workload !== workload) continue
    const key = `${t.language}/${t.mode}`
    let line = byAgent.get(key)
    if (!line) {
      const agent = AGENTS.find(a => a.language === t.language && a.mode === t.mode)
      line = {
        key,
        label: agent?.label ?? key,
        color: AGENT_COLORS[key] ?? '#888',
        points: [],
      }
      byAgent.set(key, line)
    }

    const existing = line.points.find(p => p.runId === t.run_id)
    if (existing) {
      existing.avgOps = (existing.avgOps + t.avg_ops) / 2
    } else {
      line.points.push({ runId: t.run_id, timestamp: t.timestamp, avgOps: t.avg_ops })
    }
  }

  for (const line of byAgent.values()) {
    line.points.sort((a, b) => a.timestamp.localeCompare(b.timestamp))
  }

  return Array.from(byAgent.values())
}

function drawTrendChart(
  canvas: HTMLCanvasElement,
  lines: AgentLine[],
  height: number,
) {
  const dpr = window.devicePixelRatio || 1
  const w = canvas.parentElement?.clientWidth ?? 600
  canvas.width = w * dpr
  canvas.height = height * dpr
  canvas.style.width = `${w}px`
  canvas.style.height = `${height}px`

  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.scale(dpr, dpr)

  const pad = { top: 24, right: 20, bottom: 50, left: 70 }
  const plotW = w - pad.left - pad.right
  const plotH = height - pad.top - pad.bottom

  ctx.clearRect(0, 0, w, height)

  const allTimestamps = new Set<string>()
  let maxV = 0
  for (const line of lines) {
    for (const p of line.points) {
      allTimestamps.add(p.timestamp)
      if (p.avgOps > maxV) maxV = p.avgOps
    }
  }

  const timestamps = Array.from(allTimestamps).sort()
  if (timestamps.length === 0 || maxV === 0) {
    ctx.fillStyle = '#888'
    ctx.font = '14px sans-serif'
    ctx.textAlign = 'center'
    ctx.fillText('No trend data available', w / 2, height / 2)
    return
  }

  maxV *= 1.1
  const xStep = timestamps.length > 1 ? plotW / (timestamps.length - 1) : plotW

  ctx.strokeStyle = '#333'
  ctx.lineWidth = 1
  ctx.beginPath()
  ctx.moveTo(pad.left, pad.top)
  ctx.lineTo(pad.left, pad.top + plotH)
  ctx.lineTo(pad.left + plotW, pad.top + plotH)
  ctx.stroke()

  ctx.fillStyle = '#888'
  ctx.font = '10px monospace'
  ctx.textAlign = 'center'
  for (let i = 0; i < timestamps.length; i++) {
    const x = pad.left + (timestamps.length > 1 ? i * xStep : plotW / 2)
    const dateStr = timestamps[i].replace('T', ' ').substring(5, 16)
    ctx.save()
    ctx.translate(x, pad.top + plotH + 12)
    ctx.rotate(-0.5)
    ctx.fillText(dateStr, 0, 0)
    ctx.restore()

    ctx.strokeStyle = '#1a1a1a'
    ctx.beginPath()
    ctx.moveTo(x, pad.top)
    ctx.lineTo(x, pad.top + plotH)
    ctx.stroke()
  }

  ctx.textAlign = 'right'
  const yTicks = 5
  for (let i = 0; i <= yTicks; i++) {
    const v = (maxV / yTicks) * i
    const y = pad.top + plotH - (v / maxV) * plotH
    let label: string
    if (maxV >= 10000) label = `${(v / 1000).toFixed(0)}k`
    else if (maxV >= 1000) label = `${(v / 1000).toFixed(1)}k`
    else label = v.toFixed(0)
    ctx.fillStyle = '#888'
    ctx.fillText(label, pad.left - 8, y + 4)
    ctx.strokeStyle = '#1a1a1a'
    ctx.beginPath()
    ctx.moveTo(pad.left, y)
    ctx.lineTo(pad.left + plotW, y)
    ctx.stroke()
  }

  const tsIndex = new Map<string, number>()
  timestamps.forEach((ts, i) => tsIndex.set(ts, i))

  for (const line of lines) {
    if (line.points.length < 1) continue
    ctx.strokeStyle = line.color
    ctx.lineWidth = 2
    ctx.setLineDash([])
    ctx.beginPath()
    let first = true
    for (const p of line.points) {
      const idx = tsIndex.get(p.timestamp) ?? 0
      const x = pad.left + (timestamps.length > 1 ? idx * xStep : plotW / 2)
      const y = pad.top + plotH - (p.avgOps / maxV) * plotH
      if (first) { ctx.moveTo(x, y); first = false }
      else ctx.lineTo(x, y)
    }
    ctx.stroke()

    for (const p of line.points) {
      const idx = tsIndex.get(p.timestamp) ?? 0
      const x = pad.left + (timestamps.length > 1 ? idx * xStep : plotW / 2)
      const y = pad.top + plotH - (p.avgOps / maxV) * plotH
      ctx.fillStyle = line.color
      ctx.beginPath()
      ctx.arc(x, y, 3, 0, Math.PI * 2)
      ctx.fill()
    }
  }

  let legendX = pad.left + 8
  const legendY = pad.top + 4
  ctx.font = '11px sans-serif'
  for (const line of lines) {
    ctx.strokeStyle = line.color
    ctx.lineWidth = 2
    ctx.setLineDash([])
    ctx.beginPath()
    ctx.moveTo(legendX, legendY)
    ctx.lineTo(legendX + 20, legendY)
    ctx.stroke()
    ctx.fillStyle = '#ccc'
    ctx.textAlign = 'left'
    ctx.fillText(line.label, legendX + 24, legendY + 4)
    legendX += ctx.measureText(line.label).width + 40
  }
}

const styles = {
  page: { maxWidth: 1200 } as const,
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 20,
  } as const,
  title: { fontSize: 20, fontWeight: 700 } as const,
  select: {
    background: 'var(--bg-card)',
    color: 'var(--text-primary)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--radius)',
    padding: '6px 12px',
    fontSize: 13,
  } as const,
  chart: {
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--radius-lg)',
    padding: 12,
    marginBottom: 20,
    overflow: 'hidden',
    minHeight: 400,
  } as const,
  alertBox: {
    padding: '12px 16px',
    borderRadius: 6,
    display: 'flex',
    alignItems: 'center',
    gap: 12,
    marginBottom: 8,
  } as const,
}

export function TrendsPage() {
  const { getTrends, getRegressions } = useApi()
  const [trends, setTrends] = useState<TrendPoint[]>([])
  const [regressions, setRegressions] = useState<RegressionItem[]>([])
  const [workload, setWorkload] = useState<string>('all')
  const [loading, setLoading] = useState(true)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    setLoading(true)
    Promise.all([getTrends(20), getRegressions()])
      .then(([t, r]) => {
        setTrends(t ?? [])
        setRegressions(r ?? [])
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [getTrends, getRegressions])

  const render = useCallback(() => {
    if (!canvasRef.current) return
    const lines = buildLines(trends, workload)
    drawTrendChart(canvasRef.current, lines, 400)
  }, [trends, workload])

  useEffect(() => { render() }, [render])

  useEffect(() => {
    if (!containerRef.current) return
    const observer = new ResizeObserver(() => render())
    observer.observe(containerRef.current)
    return () => observer.disconnect()
  }, [render])

  const regressionAlerts = regressions.filter(r => r.metric === 'ops_per_sec')

  return (
    <div style={styles.page}>
      <div style={styles.header}>
        <div style={styles.title}>Performance Trends</div>
        <select
          style={styles.select}
          value={workload}
          onChange={e => setWorkload(e.target.value)}
        >
          <option value="all">All Workloads</option>
          {WORKLOAD_TYPES.map(w => (
            <option key={w} value={w}>{WORKLOAD_LABELS[w as WorkloadType]}</option>
          ))}
        </select>
      </div>

      {regressionAlerts.length > 0 && (
        <div style={{ marginBottom: 20 }}>
          {regressionAlerts.map((r, i) => (
            <div
              key={i}
              style={{
                ...styles.alertBox,
                background: r.is_regression
                  ? 'rgba(255,107,107,0.08)'
                  : 'rgba(78,205,196,0.08)',
                border: `1px solid ${r.is_regression ? 'rgba(255,107,107,0.3)' : 'rgba(78,205,196,0.3)'}`,
              }}
            >
              <span style={{ fontSize: 18 }}>{r.is_regression ? '⚠️' : '✅'}</span>
              <div style={{ flex: 1 }}>
                <div style={{
                  fontWeight: 600,
                  fontSize: 13,
                  color: r.is_regression ? '#ff6b6b' : '#4ecdc4',
                }}>
                  {r.agent} — {r.workload}
                </div>
                <div style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                  {r.previous_value.toFixed(0)} → {r.current_value.toFixed(0)} ops/sec ({r.change_pct > 0 ? '+' : ''}{r.change_pct.toFixed(1)}%)
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      <div ref={containerRef} style={styles.chart}>
        {loading ? (
          <div style={{ textAlign: 'center', padding: 40, color: 'var(--text-secondary)' }}>Loading trends...</div>
        ) : (
          <canvas ref={canvasRef} />
        )}
      </div>
    </div>
  )
}
