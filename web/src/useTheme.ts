import { useEffect, useState } from 'react'

export type Theme = 'dark' | 'light'

const STORAGE_KEY = 'livesemantic-theme'

function readStoredTheme(): Theme {
  const stored = localStorage.getItem(STORAGE_KEY)
  return stored === 'light' ? 'light' : 'dark' // dark is the default, matching the mockup's own default toggle state
}

// Applies the theme as a data-theme attribute on <html> (theme.css's
// [data-theme='light'] override) and persists the choice — the mockup
// (docs/gui/mockups/, § "Hypothèses & parti pris") treats the toggle as
// a real, accessible, user-facing choice, not a hidden dev flag, so it
// needs to survive a reload.
export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setTheme] = useState<Theme>(readStoredTheme)

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    localStorage.setItem(STORAGE_KEY, theme)
  }, [theme])

  return [theme, setTheme]
}
