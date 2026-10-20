import { Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from './components/Layout'
import { ScenariosPage } from './pages/ScenariosPage'
import { LivePage } from './pages/LivePage'
import { SqlPage } from './pages/SqlPage'
import { HistoryPage } from './pages/HistoryPage'
import { ComparePage } from './pages/ComparePage'
import { TrendsPage } from './pages/TrendsPage'
import { MonitoringPage } from './pages/MonitoringPage'

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/scenarios" element={<ScenariosPage />} />
        <Route path="/live" element={<LivePage />} />
        <Route path="/sql" element={<SqlPage />} />
        <Route path="/history" element={<HistoryPage />} />
        <Route path="/compare" element={<ComparePage />} />
        <Route path="/trends" element={<TrendsPage />} />
        <Route path="/monitoring" element={<MonitoringPage />} />
        <Route path="*" element={<Navigate to="/scenarios" replace />} />
      </Routes>
    </Layout>
  )
}
