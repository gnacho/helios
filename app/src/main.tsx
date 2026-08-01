import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import './i18n'
import { applyBootPreferences } from './theme/ThemeProvider'
import App from './App.tsx'

// Preferencias (tema/densidad/reduce-motion) antes del primer render.
applyBootPreferences()

// Service worker (push): solo producción y solo en contextos seguros
// (HTTPS/localhost). En LAN HTTP navigator.serviceWorker es undefined.
if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {})
  })
}

// Sin StrictMode: duplicaría los efectos de canvas/animación.
createRoot(document.getElementById('root')!).render(
  <BrowserRouter>
    <App />
  </BrowserRouter>,
)
