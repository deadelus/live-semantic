import { useEffect, useRef, useState } from 'react'
import {
  createSession,
  listDevices,
  listSessions,
  removeSession,
  type DeviceInfo,
  type SessionInfo,
} from './api'
import { Button } from './components/Button'
import { StatusBadge } from './components/StatusBadge'
import { useToast } from './toast/ToastProvider'
import './SourcesList.css'

interface SourcesListProps {
  onOpen: (session: SessionInfo) => void
}

// Home screen — vue liste (docs/gui/mockups/ screen 1b). The mosaic view
// (1a/1e) isn't built here: it needs a dedicated low-fps preview WS
// protocol per tile (docs/gui/spec.md § 3.1) that doesn't exist yet —
// building it against /ws/sessions/:id's full-rate protocol would waste
// bandwidth per tile for no reason, not a shortcut worth taking.
//
// Device picker — redesigned 2026-08-13, replacing the earlier "caméra
// serveur" / "caméra navigateur" dropdown: that let two sources silently
// fight over the exact same physical webcam with no way to see it coming
// (the bug behind TODO.md § H1's "Échecs d'input silencieux" fix). Now
// GET /api/v1/devices lists real local camera indices with a `busy` flag
// (a running session already claims it) — busy devices are shown
// disabled instead of letting the user hit that conflict again. Browser
// camera capture (getUserMedia, api.ts's `Source.kind === 'browser'`)
// still exists and works server-side (BrowserInput, /ws/sessions/:id/ingest)
// but isn't exposed here anymore — it solves a different problem (a
// remote device's own camera, not a local index) that doesn't fit a
// device picker; revisit as its own entry point if/when that's needed.
//
// Both lists poll independently (2s) rather than pushing over WS — no
// aggregated "changed" event exists server-side for either.
export function SourcesList({ onOpen }: SourcesListProps) {
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [devices, setDevices] = useState<DeviceInfo[]>([])
  const [busyIndex, setBusyIndex] = useState<number | null>(null)
  const { pushError } = useToast()
  const toastedErrors = useRef<Record<string, string>>({})

  useEffect(() => {
    const poll = () => {
      listSessions()
        .then((r) => {
          setSessions(r.sessions)
          for (const s of r.sessions) {
            if (s.error && toastedErrors.current[s.id] !== s.error) {
              toastedErrors.current[s.id] = s.error
              pushError('Une source a rencontré une erreur.')
            }
          }
        })
        .catch(() => {
          /* transient network hiccup — next poll will retry */
        })
    }
    poll()
    const id = setInterval(poll, 2000)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const [devicesLoading, setDevicesLoading] = useState(false)

  // Scan once on mount, never on an interval — real bug found 2026-08-14
  // in a live run: input.ProbeDevices (implementation/streamer/input/
  // probe.go) *physically opens and closes every non-busy camera index*
  // (no portable "list connected webcams" API exists, see its own doc
  // comment) to check availability. Polling GET /api/v1/devices every 2s
  // like the sessions list above meant every non-busy camera's hardware
  // LED flickered on/off every 2 seconds, and — worse — a Continuity
  // Camera (iPhone) index being probed repeatedly kept re-triggering
  // iOS's "authorize this Mac to use your camera" handoff prompt in a
  // loop, unprompted by any user action. refreshDevices below is a
  // manual re-scan (button) instead — same probing cost, but the user
  // decides when to pay it (e.g. after plugging in a new camera), not a
  // background timer.
  const refreshDevices = () => {
    setDevicesLoading(true)
    listDevices()
      .then((r) => setDevices(r.devices))
      .catch(() => pushError('Impossible de lister les caméras.'))
      .finally(() => setDevicesLoading(false))
  }

  useEffect(refreshDevices, []) // eslint-disable-line react-hooks/exhaustive-deps

  const handleAdd = async (index: number) => {
    setBusyIndex(index)
    try {
      const info = await createSession({ kind: 'local', device: index })
      setSessions((prev) => [...prev, info])
    } catch {
      pushError("Impossible d'ajouter cette caméra.")
    } finally {
      setBusyIndex(null)
    }
  }

  const handleRemove = async (id: string) => {
    try {
      await removeSession(id)
      setSessions((prev) => prev.filter((s) => s.id !== id))
    } catch {
      pushError('Impossible de supprimer cette source.')
    }
  }

  return (
    <div className="ls-sources">
      <div className="ls-sources__devices">
        <div className="ls-sources__devices-header">
          <h2>Caméras disponibles</h2>
          <Button variant="discrete" onClick={refreshDevices} disabled={devicesLoading}>
            {devicesLoading ? 'Scan…' : '🔄 Actualiser'}
          </Button>
        </div>
        {devices.length === 0 ? (
          <p className="ls-muted">Aucune caméra détectée.</p>
        ) : (
          <div className="ls-sources__device-list">
            {devices.map((d) => (
              <Button
                key={d.index}
                variant="primary"
                onClick={() => handleAdd(d.index)}
                disabled={d.busy || busyIndex === d.index}
              >
                {d.busy ? `Caméra ${d.index} (occupée)` : `+ Caméra ${d.index}`}
              </Button>
            ))}
          </div>
        )}
      </div>

      {sessions.length === 0 ? (
        <p className="ls-muted">Aucune source ajoutée — choisissez une caméra ci-dessus.</p>
      ) : (
        <ul className="ls-sources__list">
          {sessions.map((s) => (
            <li key={s.id} className="ls-sources__row">
              <StatusBadge status={s.error ? 'error' : s.running ? 'connected' : 'disconnected'} />
              <span className="ls-sources__label">Caméra {s.source.device ?? 0}</span>
              <span className="ls-sources__filter">{s.filter || '—'}</span>
              <div className="ls-sources__actions">
                <Button variant="secondary" onClick={() => onOpen(s)}>
                  Ouvrir
                </Button>
                <Button variant="danger" onClick={() => handleRemove(s.id)}>
                  Supprimer
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
