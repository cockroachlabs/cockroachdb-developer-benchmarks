import { useState, useCallback } from 'react'
import { RunHistoryTable } from '../components/RunHistoryTable'
import { CompareView } from '../components/CompareView'
import { useApi } from '../hooks/useApi'

const styles = {
  title: {
    fontSize: 18,
    fontWeight: 600,
    marginBottom: 16,
  } as const,
  hint: {
    fontSize: 12,
    color: 'var(--text-secondary)',
    marginBottom: 12,
  } as const,
  btnReport: {
    padding: '8px 16px',
    background: 'var(--accent)',
    color: '#0a0a0a',
    borderRadius: 'var(--radius)',
    fontWeight: 600,
    fontSize: 13,
    cursor: 'pointer',
    border: 'none',
    marginBottom: 16,
    marginLeft: 12,
  } as const,
}

export function ComparePage() {
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const { compareReport } = useApi();

  const handleCompareSelect = useCallback((ids: string[]) => {
    setSelectedIds(ids);
  }, []);

  const handleGenerateReport = async () => {
    try {
      await compareReport(selectedIds);
    } catch (_) {}
  };

  return (
    <div>
      <div style={styles.title}>Compare Runs</div>
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <div style={styles.hint}>
          Select 2 or more runs to compare
          {selectedIds.length > 0 && ` (${selectedIds.length} selected)`}
        </div>
        {selectedIds.length >= 2 && (
          <button style={styles.btnReport} onClick={handleGenerateReport}>
            Generate Combined Report
          </button>
        )}
      </div>

      <RunHistoryTable selectable onCompareSelect={handleCompareSelect} />

      {selectedIds.length >= 2 && (
        <div style={{ marginTop: 24 }}>
          <CompareView runIds={selectedIds} />
        </div>
      )}
    </div>
  )
}
