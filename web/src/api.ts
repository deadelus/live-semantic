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

// --- Bibliothèque — Termes (gallery) + Collections (docs/gui/design-brief.md
// § Bibliothèque, backend added 2026-08-13, branch feat/gui-library-backend).
// Mirrors uc.GalleryEntryInfo/uc.CollectionInfo — kept as separate,
// frontend-owned types rather than importing anything (there's nothing to
// import across the Go/TS boundary), same spirit as SessionInfo above. ---

export interface GalleryImageRef {
  id: string
}

// Mirrors uc.GalleryEntryInfo. A Term with zero Images can't actually
// happen server-side (storage.GalleryStorage.RemoveImage deletes the
// whole entry once its last photo is gone, "un Terme sans photo n'existe
// pas") — Images is still typed as possibly-empty here defensively, not
// because the backend is expected to ever send that.
export interface GalleryEntry {
  name: string
  enabled: boolean
  images: GalleryImageRef[]
  cocoClass?: string
}

export function listGallery(): Promise<{ entries: GalleryEntry[] }> {
  return fetch('/api/v1/gallery').then((res) => parseJSONOrThrow<{ entries: GalleryEntry[] }>(res))
}

export function removeGalleryTerm(name: string): Promise<{ status: string }> {
  return fetch(`/api/v1/gallery/${encodeURIComponent(name)}`, { method: 'DELETE' }).then((res) =>
    parseJSONOrThrow<{ status: string }>(res),
  )
}

export function removeGalleryImage(name: string, imageID: string): Promise<{ status: string }> {
  return fetch(
    `/api/v1/gallery/${encodeURIComponent(name)}/images/${encodeURIComponent(imageID)}`,
    { method: 'DELETE' },
  ).then((res) => parseJSONOrThrow<{ status: string }>(res))
}

// updateGalleryTerm applies any of new_name/enabled/coco_class at once —
// mirrors galleryController.update's own "applied in that order" contract
// (rename, then enabled, then coco_class). Pass cocoClass: '' to clear an
// existing link (see gallery.go's own doc comment — a nil field is "leave
// untouched", an empty string is "clear it", the two are not the same).
export function updateGalleryTerm(
  name: string,
  patch: { newName?: string; enabled?: boolean; cocoClass?: string },
): Promise<{ status: string; name: string }> {
  return fetch(`/api/v1/gallery/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      new_name: patch.newName,
      enabled: patch.enabled,
      coco_class: patch.cocoClass,
    }),
  }).then((res) => parseJSONOrThrow<{ status: string; name: string }>(res))
}

// Mirrors uc.CollectionInfo.
export interface CollectionEntry {
  name: string
  tags: string[]
  terms: string[]
}

export function listCollections(): Promise<{ collections: CollectionEntry[] }> {
  return fetch('/api/v1/collections').then((res) =>
    parseJSONOrThrow<{ collections: CollectionEntry[] }>(res),
  )
}

export function createCollection(
  name: string,
  tags: string[],
): Promise<{ status: string; name: string }> {
  return fetch('/api/v1/collections', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, tags }),
  }).then((res) => parseJSONOrThrow<{ status: string; name: string }>(res))
}

export function removeCollection(name: string): Promise<{ status: string }> {
  return fetch(`/api/v1/collections/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(
    (res) => parseJSONOrThrow<{ status: string }>(res),
  )
}

export function updateCollection(
  name: string,
  patch: { newName?: string; tags?: string[] },
): Promise<{ status: string; name: string }> {
  return fetch(`/api/v1/collections/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ new_name: patch.newName, tags: patch.tags }),
  }).then((res) => parseJSONOrThrow<{ status: string; name: string }>(res))
}

export function addTermToCollection(
  collectionName: string,
  termName: string,
): Promise<{ status: string }> {
  return fetch(
    `/api/v1/collections/${encodeURIComponent(collectionName)}/terms/${encodeURIComponent(termName)}`,
    { method: 'POST' },
  ).then((res) => parseJSONOrThrow<{ status: string }>(res))
}

export function removeTermFromCollection(
  collectionName: string,
  termName: string,
): Promise<{ status: string }> {
  return fetch(
    `/api/v1/collections/${encodeURIComponent(collectionName)}/terms/${encodeURIComponent(termName)}`,
    { method: 'DELETE' },
  ).then((res) => parseJSONOrThrow<{ status: string }>(res))
}
