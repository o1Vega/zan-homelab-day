import React, { useEffect, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/day.css'
import './styles/app.css'
import './styles/settings.css'
import './styles/memories.css'
import './styles/fx.css'
import './styles/social.css'
import { AppProvider, useApp } from './lib/app-context'
import { I18nProvider } from './lib/i18n'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import { ShortcutsSheet } from './components/ShortcutsSheet'
import Login from './pages/Login'
import Day from './pages/Day'
import Calendar from './pages/Calendar'
import Journal from './pages/Journal'
import Year from './pages/Year'
import Settings from './pages/Settings'
import Ask from './pages/Ask'
import Stories from './pages/Stories'
import Memories from './pages/Memories'
import NewMemory from './pages/NewMemory'
import MemoryPage from './pages/Memory'
import Posts from './pages/Posts'
import Landing from './pages/Landing'
import Post from './pages/Post'
import People from './pages/People'
import Tokonoma from './pages/Tokonoma'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/" replace />
  return <>{children}</>
}

/** `/` for guests: the landing; the first user goes to setup; anyone signed in gets the day in the app shell. */
function Root() {
  const { status, user } = useApp()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Landing />
  return <ErrorBoundary where="day"><Layout><Day /></Layout></ErrorBoundary>
}

/** The keyboard shortcuts sheet, from anywhere (the shell, Settings, the palette). */
function Shortcuts() {
  const [open, setOpen] = useState(false)
  useEffect(() => {
    const h = () => setOpen(true)
    window.addEventListener('zan:shortcuts', h)
    return () => window.removeEventListener('zan:shortcuts', h)
  }, [])
  return open ? <ShortcutsSheet onClose={() => setOpen(false)} /> : null
}

function App() {
  return (
    <>
      <Routes>
        <Route path="/login" element={<Login mode="login" />} />
        <Route path="/setup" element={<Login mode="setup" />} />
        <Route path="/register" element={<Login mode="register" />} />
        <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><Settings /></ErrorBoundary></Gate>} />
        <Route path="/" element={<Root />} />
        <Route element={<Gate><ErrorBoundary where="day"><Layout /></ErrorBoundary></Gate>}>
          <Route path="/memories" element={<Memories />} />
          <Route path="/memory/new" element={<NewMemory />} />
          <Route path="/memory/:id" element={<MemoryPage />} />
          <Route path="/posts" element={<Posts />} />
          <Route path="/post/:id" element={<Post />} />
          <Route path="/people" element={<People />} />
          <Route path="/people/:id" element={<Tokonoma />} />
          <Route path="/day/:date" element={<Day />} />
          <Route path="/calendar" element={<Calendar />} />
          <Route path="/calendar/:month" element={<Calendar />} />
          <Route path="/journal" element={<Journal />} />
          <Route path="/year" element={<Year />} />
          <Route path="/year/:year" element={<Year />} />
          <Route path="/ask" element={<Ask />} />
          <Route path="/stories" element={<Stories />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
      <Shortcuts />
    </>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><I18nProvider><AppProvider><App /></AppProvider></I18nProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
