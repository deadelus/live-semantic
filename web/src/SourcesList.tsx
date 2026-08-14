import { useEffect, useMemo, useRef, useState } from 'react'
import {
  createSession,
  listDevices,
  listSessions,
  removeSession,
  sourceLabel,
  sourceType,
  type DeviceInfo,
  type SessionInfo,
} from './api'
import { Button } from './components/Button'
import { useToast } from './toast/ToastProvider'
import './SourcesList.css'

interface SourcesListProps {
  onOpen: (session: SessionInfo) => void
}

// Home screen — vue liste (docs/gui/mockups/ screen 1b), rebuilt
// 2026-08-14 as a real table matching the mockup's own grid columns
// (dot, Source, Type, Définition, Filtres actifs, Dernier événement,
// Actions). Two columns intentionally show "—", not invented data:
// "Définition" (resolution) and "Dernier événement" have no backend
// source yet — session.Info carries neither a frame size nor an
// aggregated event log (docs/gui/spec.md § 3.5 "Historique/alertes" is
// still "zéro ligne de code"). Showing a placeholder honestly is better
// than fabricating a number nobody measured. The mockup's own bottom
// "Journal des événements" drawer is the same story — not built here for
// the same reason.
//
// The mosaic view (1a/1e) isn't built here either: it needs a dedicated
// low-fps preview WS protocol per tile (docs/gui/spec.md § 3.1) that
// doesn't exist yet — the toggle below is shown but disabled, not
// hidden, so the gap is visible rather than silently absent.
//
// Device picker — redesigned 2026-08-13 (see refreshDevices' own doc
// comment for the polling-frequency bug fixed 2026-08-14): GET
// /api/v1/devices lists real local camera indices with a `busy` flag (a
// running session already claims it) — busy devices are shown disabled.
export function SourcesList({ onOpen }: SourcesListProps) {
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [devices, setDevices] = useState<DeviceInfo[]>([])
  const [busyIndex, setBusyIndex] = useState<number | null>(null)
  const [search, setSearch] = useState('')
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

  const connectedCount = sessions.filter((s) => s.running).length
  const visibleSessions = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return sessions
    return sessions.filter((s) => sourceLabel(s).toLowerCase().includes(q))
  }, [sessions, search])

  const [addTab, setAddTab] = useState<'local' | 'file' | 'webrtc'>('local')
  const [fileURI, setFileURI] = useState('')
  const [fileBusy, setFileBusy] = useState(false)
  const [webrtcBusy, setWebrtcBusy] = useState(false)

  // handleAddFile creates a "file" session — covers a local video file
  // path AND rtsp://.../http(s):// URLs, resolved identically by gocv
  // (input.FileInput's own doc comment): no separate RTSP tab needed.
  // Backend support for this already existed (sessionInputFactory, since
  // 2026-08-11) — this was purely a missing frontend trigger, no backend
  // work needed.
  const handleAddFile = async () => {
    const uri = fileURI.trim()
    if (!uri) return
    setFileBusy(true)
    try {
      const info = await createSession({ kind: 'file', uri })
      setSessions((prev) => [...prev, info])
      setFileURI('')
    } catch {
      pushError("Impossible d'ajouter cette source (chemin/URL invalide ?).")
    } finally {
      setFileBusy(false)
    }
  }

  // handleAddWebRTC only *creates* the session (Source.Kind: "webrtc") —
  // the actual RTCPeerConnection/getUserMedia negotiation
  // (WebRTCCamera.ts) happens once the user opens it and clicks
  // "Démarrer" (LiveView.tsx), same lazy pattern already used for
  // "browser" kind sessions. Backend added 2026-08-13 (branch
  // feat/gui-webrtc-ingestion), this was the missing frontend trigger.
  const handleAddWebRTC = async () => {
    setWebrtcBusy(true)
    try {
      const info = await createSession({ kind: 'webrtc' })
      setSessions((prev) => [...prev, info])
    } catch {
      pushError("Impossible d'ajouter cette source WebRTC.")
    } finally {
      setWebrtcBusy(false)
    }
  }

  return (
    <div className="ls-sources">
      <div className="ls-sources__devices">
        <div className="ls-sources__devices-header">
          <h2>+ Ajouter une source</h2>
          {addTab === 'local' && (
            <Button variant="discrete" onClick={refreshDevices} disabled={devicesLoading}>
              {devicesLoading ? 'Scan…' : '🔄 Actualiser'}
            </Button>
          )}
        </div>

        <div className="ls-sources__add-tabs">
          <button
            className={`ls-sources__add-tab ${addTab === 'local' ? 'ls-sources__add-tab--active' : ''}`}
            onClick={() => setAddTab('local')}
          >
            Caméra locale
          </button>
          <button
            className={`ls-sources__add-tab ${addTab === 'file' ? 'ls-sources__add-tab--active' : ''}`}
            onClick={() => setAddTab('file')}
          >
            Fichier / RTSP / HTTP
          </button>
          <button
            className={`ls-sources__add-tab ${addTab === 'webrtc' ? 'ls-sources__add-tab--active' : ''}`}
            onClick={() => setAddTab('webrtc')}
          >
            Caméra WebRTC
          </button>
        </div>

        {addTab === 'local' &&
          (devices.length === 0 ? (
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
          ))}

        {addTab === 'file' && (
          <div className="ls-sources__add-form">
            <input
              className="ls-input"
              type="text"
              value={fileURI}
              onChange={(e) => setFileURI(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleAddFile()}
              placeholder="/chemin/vers/video.mp4, rtsp://…, http://…"
            />
            <Button variant="primary" onClick={handleAddFile} disabled={fileBusy || !fileURI.trim()}>
              + Ajouter
            </Button>
          </div>
        )}

        {addTab === 'webrtc' && (
          <div className="ls-sources__add-form">
            <p className="ls-muted">
              Négociation WebRTC sans serveur TURN — fonctionne sur le même réseau local, pas
              forcément derrière un NAT distant.
            </p>
            <Button variant="primary" onClick={handleAddWebRTC} disabled={webrtcBusy}>
              + Ajouter une caméra WebRTC
            </Button>
          </div>
        )}
      </div>

      <div className="ls-sources__toolbar">
        <div className="ls-sources__view-toggle">
          <span className="ls-sources__view-toggle-active">Liste</span>
          <span className="ls-sources__view-toggle-disabled" title="Pas encore disponible — docs/gui/spec.md § 3.1">
            Mosaïque
          </span>
        </div>
        <span className="ls-sources__count">
          {sessions.length} source{sessions.length > 1 ? 's' : ''} · {connectedCount} connectée
          {connectedCount > 1 ? 's' : ''}
        </span>
        <div className="ls-topbar__spacer" />
        <input
          className="ls-input ls-sources__search"
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Rechercher une source…"
        />
      </div>

      {visibleSessions.length === 0 ? (
        <p className="ls-muted">
          {sessions.length === 0
            ? 'Aucune source ajoutée — choisissez une caméra ci-dessus.'
            : 'Aucune source ne correspond à la recherche.'}
        </p>
      ) : (
        <div className="ls-sources__table">
          <div className="ls-sources__row ls-sources__row--header">
            <span />
            <span>Source</span>
            <span>Type</span>
            <span>Définition</span>
            <span>Filtres actifs</span>
            <span>Dernier événement</span>
            <span className="ls-sources__actions-header">Actions</span>
          </div>
          {visibleSessions.map((s) => {
            const status = s.error ? 'error' : s.running ? 'connected' : 'disconnected'
            return (
              <div key={s.id} className="ls-sources__row">
                <span className={`ls-sources__dot ls-sources__dot--${status}`} />
                <span className="ls-sources__name">
                  {sourceLabel(s)}
                  <span className={`ls-sources__status-text ls-sources__status-text--${status}`}>
                    {s.error ? 'erreur' : s.running ? 'connecté' : 'déconnecté'}
                  </span>
                </span>
                <span className="ls-sources__mono">{sourceType(s)}</span>
                <span className="ls-sources__mono">—</span>
                <span className="ls-sources__filter">{s.filter || '—'}</span>
                <span className="ls-sources__last">—</span>
                <span className="ls-sources__actions">
                  <Button variant="secondary" onClick={() => onOpen(s)}>
                    Ouvrir
                  </Button>
                  <Button variant="discrete" disabled title="Pas encore disponible">
                    Éditer
                  </Button>
                  <Button variant="danger" onClick={() => handleRemove(s.id)}>
                    Suppr.
                  </Button>
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
