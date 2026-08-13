// Thin wrapper around the multi-flux REST control surface
// (docs/gui/spec.md § 1.1, internal/transport/adapters/api/sessions.go +
// session.go). Migrated 2026-08-13 from the H1 mono-session surface
// (/api/v1/recognition/*, still alive server-side but no longer used
// here) to /api/v1/sessions/* — this is the foundation every later
// multi-source screen (mosaic, sources list, TODO.md § H2) builds on, so
// it's done first rather than bolted onto a single hardcoded session
// later.

export type SourceKind = 'local' | 'browser'

// Mirrors session.Source (internal/application/session/session.go) —
// Device only makes sense for "local" (which webcam index), URI only for
// a future "file"/RTSP kind (not wired in the GUI yet, TODO.md § H1).
export interface Source {
  kind: SourceKind
  device?: number
  uri?: string
}

// Mirrors session.Info.
export interface SessionInfo {
  id: string
  source: Source
  running: boolean
  filter?: string
  error?: string
}

async function parseJSONOrThrow<T>(res: Response): Promise<T> {
  const body = await res.json().catch(() => null)
  if (!res.ok) {
    // The backend always returns {"error": "..."} on a non-2xx response —
    // surfaced as-is rather than a generic "request failed", since the
    // message is usually actionable (e.g. "session not found").
    const message = body?.error ?? `request failed (${res.status})`
    throw new Error(message)
  }
  return body as T
}

// createSession must be called before the video/ingest WebSocket is
// opened (docs/gui/spec.md § 1.1's own note: "recognition doesn't start
// automatically ... once it's ready, e.g. after opening the
// /ws/sessions/:id video connection so no frames are missed").
export function createSession(source: Source): Promise<SessionInfo> {
  return fetch('/api/v1/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source }),
  }).then((res) => parseJSONOrThrow<SessionInfo>(res))
}

export function getSession(id: string): Promise<SessionInfo> {
  return fetch(`/api/v1/sessions/${id}`).then((res) => parseJSONOrThrow<SessionInfo>(res))
}

export function listSessions(): Promise<{ sessions: SessionInfo[] }> {
  return fetch('/api/v1/sessions').then((res) => parseJSONOrThrow<{ sessions: SessionInfo[] }>(res))
}

// removeSession is best-effort cleanup (e.g. on unmount/source switch) —
// callers shouldn't block user-visible actions on it succeeding.
export function removeSession(id: string): Promise<{ status: string }> {
  return fetch(`/api/v1/sessions/${id}`, { method: 'DELETE' }).then((res) =>
    parseJSONOrThrow<{ status: string }>(res),
  )
}

export function startSessionRecognition(
  id: string,
  filter: string,
): Promise<{ status: string; filter: string }> {
  return fetch(`/api/v1/sessions/${id}/recognition/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filter }),
  }).then((res) => parseJSONOrThrow<{ status: string; filter: string }>(res))
}

export function stopSessionRecognition(id: string): Promise<{ status: string }> {
  return fetch(`/api/v1/sessions/${id}/recognition/stop`, { method: 'POST' }).then((res) =>
    parseJSONOrThrow<{ status: string }>(res),
  )
}

// Mirrors api/devices.go's deviceInfo — best-effort local camera
// discovery (index-probing, not a true OS enumeration, see that file's
// doc comment). busy=true means a running session already claims this
// device — the GUI greys those out rather than letting a second session
// silently fight over the same physical webcam (the real bug this
// replaced, TODO.md § H1 "Échecs d'input silencieux").
export interface DeviceInfo {
  index: number
  busy: boolean
}

export function listDevices(): Promise<{ devices: DeviceInfo[] }> {
  return fetch('/api/v1/devices').then((res) => parseJSONOrThrow<{ devices: DeviceInfo[] }>(res))
}
