// Client de l'API Nodra : cookie de session, garde CSRF (X-Requested-With), erreurs en français.

export class ApiError extends Error {
  status: number
  body: unknown
  constructor(status: number, message: string, body?: unknown) {
    super(message)
    this.status = status
    this.body = body
  }
}

export const API_BASE = "/api/v1"

export async function api<T = unknown>(path: string, init: { method?: string; json?: unknown } = {}): Promise<T> {
  const res = await fetch(API_BASE + path, {
    method: init.method ?? "GET",
    credentials: "include",
    headers: {
      Accept: "application/json",
      "X-Requested-With": "nodra",
      ...(init.json !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: init.json !== undefined ? JSON.stringify(init.json) : undefined,
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  let data: unknown
  try {
    data = text ? JSON.parse(text) : undefined
  } catch {
    data = text
  }
  if (!res.ok) {
    const msg = (data as { error?: unknown } | undefined)?.error
    throw new ApiError(res.status, typeof msg === "string" && msg ? msg : `Erreur HTTP ${res.status}`, data)
  }
  return data as T
}

export const get = <T>(path: string) => api<T>(path)
export const post = <T>(path: string, json: unknown = {}) => api<T>(path, { method: "POST", json })

export const teamPath = (team: string, path = "") => `/teams/${team}${path}`
