import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { AuthProvider } from './auth'
import { Router } from './router'
import { I18nProvider } from './i18n'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <I18nProvider>
      <Router>
        <AuthProvider>
          <App />
        </AuthProvider>
      </Router>
    </I18nProvider>
  </StrictMode>,
)
