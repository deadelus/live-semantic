import './StatusBadge.css'

// Matches docs/gui/mockups/ screen 1h's "Badges de statut" set — 4 states.
// "reconnecting" pulses (ls-pulse, theme.css) — the other three are static.
export type Status = 'connected' | 'reconnecting' | 'error' | 'disconnected'

const LABELS: Record<Status, string> = {
  connected: 'Connecté',
  reconnecting: 'Reconnexion…',
  error: 'Erreur',
  disconnected: 'Déconnecté',
}

export function StatusBadge({ status }: { status: Status }) {
  return (
    <span className={`ls-status-badge ls-status-badge--${status}`}>
      <span className="ls-status-badge__dot" />
      {LABELS[status]}
    </span>
  )
}
