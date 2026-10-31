import { afterEach, describe, expect, it, vi } from "vitest"
import { api, ApiError, buildURL } from "./api"

afterEach(() => vi.unstubAllGlobals())

function mockFetch(status: number, body?: unknown) {
  const fn = vi.fn(async () => new Response(body === undefined ? null : JSON.stringify(body), { status }))
  vi.stubGlobal("fetch", fn)
  return fn
}

describe("api", () => {
  it("envoie le JSON avec la garde anti-CSRF et le cookie", async () => {
    const fn = mockFetch(200, { ok: true })
    await expect(api("/x", { method: "POST", json: { a: 1 } })).resolves.toEqual({ ok: true })
    const [url, init] = fn.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe("/api/v1/x")
    expect(init.credentials).toBe("include")
    expect((init.headers as Record<string, string>)["X-Requested-With"]).toBe("nodra")
    expect((init.headers as Record<string, string>)["Content-Type"]).toBe("application/json")
    expect(init.body).toBe('{"a":1}')
  })

  it("lève ApiError avec le message du backend", async () => {
    mockFetch(409, { error: "déjà utilisé" })
    await expect(api("/x")).rejects.toMatchObject({ status: 409, message: "déjà utilisé" })
    mockFetch(403)
    await expect(api("/x")).rejects.toBeInstanceOf(ApiError)
  })

  it("renvoie undefined pour 204", async () => {
    mockFetch(204)
    await expect(api("/x", { method: "DELETE" })).resolves.toBeUndefined()
  })

  it("construit la query string sans valeurs vides", () => {
    expect(buildURL("/r", { a: "1", b: "", c: undefined, d: 0 })).toBe("/api/v1/r?a=1&d=0")
  })
})
