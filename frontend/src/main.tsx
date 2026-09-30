import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import App from './App.tsx'
import { initializeAccountFromURL } from './lib/accounts'

initializeAccountFromURL()

// Standalone SPA entry. BrowserRouter drives client-side routing; react-aria's
// RouterProvider is wired inside <App> so HeroUI links/sidebar items navigate
// through the router. HeroUI v3 needs no global provider.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
)
