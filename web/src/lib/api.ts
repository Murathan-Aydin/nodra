// Client de l'API Nodra : cookie de session, garde anti-CSRF, erreurs { error } en français.

export class ApiError extends Error {
  status: number
  body: unknown
  constructor(status: number, message: string, body?: unknown) {
    super(message)
    this.status = status
    this.body = body
  }
}

export type RequestOptions = Omit<RequestInit, "body"> & {
  /** Corps JSON (sérialisé) ; sinon `body` brut (fichier, flux). */
  json?: unknown
  body?: BodyInit | null
  query?: Record<string, string | number | boolean | undefined | null>
}

export const API_BASE = "/api/v1"

export function buildURL(path: string, query?: RequestOptions["query"]): string {
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v !== undefined && v !== null && v !== "") qs.set(k, String(v))
  }
  const q = qs.toString()
  return `${API_BASE}${path}${q ? (path.includes("?") ? "&" : "?") + q : ""}`
}

/** Appelle l'API et renvoie le JSON décodé ; lève ApiError (status, message du backend). */
export async function api<T = unknown>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { json, query, headers, body, ...rest } = opts
  const res = await fetch(buildURL(path, query), {
    credentials: "include",
    ...rest,
    headers: {
      Accept: "application/json",
      "X-Requested-With": "nodra",
      ...(json !== undefined ? { "Content-Type": "application/json" } : {}),
      ...headers,
    },
    body: json !== undefined ? JSON.stringify(json) : body,
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  let data: unknown = undefined
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }
  if (!res.ok) {
    const backend = (data as { error?: unknown } | undefined)?.error
    const message =
      typeof backend === "string" && backend ? backend : res.status === 403 ? "Droits insuffisants" : `Erreur HTTP ${res.status}`
    throw new ApiError(res.status, message, data)
  }
  return data as T
}

export const get = <T>(path: string, query?: RequestOptions["query"]) => api<T>(path, { query })
export const post = <T>(path: string, json?: unknown) => api<T>(path, { method: "POST", json: json ?? {} })
export const patch = <T>(path: string, json: unknown) => api<T>(path, { method: "PATCH", json })
export const put = <T>(path: string, json: unknown) => api<T>(path, { method: "PUT", json })
export const del = <T>(path: string, json?: unknown) => api<T>(path, { method: "DELETE", json })

/** Chemin d'une route d'équipe. */
export const teamPath = (team: string, path = "") => `/teams/${team}${path}`

/** URL WebSocket (même origine) d'une route de l'API. */
export function wsURL(path: string, query?: RequestOptions["query"]): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:"
  return `${proto}//${location.host}${buildURL(path, query)}`
}
