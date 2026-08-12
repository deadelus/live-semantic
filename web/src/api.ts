// Thin wrapper around the H1 backend's REST control surface
// (docs/gui/spec.md § 2, internal/transport/adapters/api). H1 minimal
// scope: one session for the whole backend process, no session ID yet
// (TODO.md § H1 "Multi-flux" is the follow-up that adds one) — every call
// here controls that single shared session.

export interface RecognitionStatus {
  running: boolean
}

async function parseJSONOrThrow<T>(res: Response): Promise<T> {
  const body = await res.json().catch(() => null)
  if (!res.ok) {
    // The backend always returns {"error": "..."} on a non-2xx response
    // (recognition.go) — surfaced as-is rather than a generic "request
    // failed", since the message is usually actionable (e.g. "already
    // running", "invalid filter").
    const message = body?.error ?? `request failed (${res.status})`
    throw new Error(message)
  }
  return body as T
}

export function startRecognition(filter: string): Promise<{ status: string; filter: string }> {
  return fetch('/api/v1/recognition/start', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filter }),
  }).then((res) => parseJSONOrThrow<{ status: string; filter: string }>(res))
}

export function stopRecognition(): Promise<{ status: string }> {
  return fetch('/api/v1/recognition/stop', { method: 'POST' }).then((res) =>
    parseJSONOrThrow<{ status: string }>(res),
  )
}

export function getStatus(): Promise<RecognitionStatus> {
  return fetch('/api/v1/recognition/status').then((res) => parseJSONOrThrow<RecognitionStatus>(res))
}
