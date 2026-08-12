import { useTheme } from '../useTheme'
import './ThemeToggle.css'

// Matches docs/gui/mockups/ screens' top-bar theme switch — a real,
// always-visible, accessible toggle (TODO.md § H2: "pas enfoui"), not a
// hidden setting.
export function ThemeToggle() {
  const [theme, setTheme] = useTheme()
  return (
    <div className="ls-theme-toggle" role="group" aria-label="Thème">
      <button
        type="button"
        className={theme === 'dark' ? 'active' : ''}
        onClick={() => setTheme('dark')}
        aria-pressed={theme === 'dark'}
      >
        Sombre
      </button>
      <button
        type="button"
        className={theme === 'light' ? 'active' : ''}
        onClick={() => setTheme('light')}
        aria-pressed={theme === 'light'}
      >
        Clair
      </button>
    </div>
  )
}
