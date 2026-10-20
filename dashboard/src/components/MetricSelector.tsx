import { METRIC_OPTIONS, type MetricKey } from '../types'

interface Props {
  value: MetricKey;
  onChange: (key: MetricKey) => void;
}

export function MetricSelector({ value, onChange }: Props) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value as MetricKey)}
      style={{ minWidth: 160 }}
    >
      {METRIC_OPTIONS.map((opt) => (
        <option key={opt.value} value={opt.value}>
          {opt.direction === 'higher' ? '↑' : '↓'} {opt.label} {opt.unit && `(${opt.unit})`}
        </option>
      ))}
    </select>
  )
}
