import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import Layout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import CalendarPage from './pages/CalendarPage'
import Board from './pages/Board'
import OutreachCenter from './pages/OutreachCenter'
import Drafts from './pages/Drafts'
import Vendors from './pages/Vendors'
import Preferred from './pages/Preferred'
import Properties from './pages/Properties'
import Intake from './pages/Intake'
import WorkOrderDetail from './pages/WorkOrderDetail'
import TicketDetail from './pages/TicketDetail'
import { getSession } from './lib/api'

function RequireAuth({ children }: { children: React.ReactNode }) {
  if (!getSession()) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          element={
            <RequireAuth>
              <Layout />
            </RequireAuth>
          }
        >
          <Route path="/" element={<Dashboard />} />
          <Route path="/calendar" element={<CalendarPage />} />
          <Route path="/board" element={<Board />} />
          <Route path="/outreach" element={<OutreachCenter />} />
          <Route path="/drafts" element={<Drafts />} />
          <Route path="/vendors" element={<Vendors />} />
          <Route path="/preferred" element={<Preferred />} />
          <Route path="/properties" element={<Properties />} />
          <Route path="/intake" element={<Intake />} />
          <Route path="/work-orders/:id" element={<WorkOrderDetail />} />
          <Route path="/tickets/:type/:id" element={<TicketDetail />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
