import { describe, expect, it } from "vitest"
import { bytes, duration, relativeTime } from "./format"

describe("format", () => {
  it("affiche les tailles en unités binaires", () => {
    expect(bytes(512)).toBe("512 o")
    expect(bytes(1536)).toBe("1,5 Kio")
    expect(bytes(3 * 1024 ** 3)).toBe("3 Gio")
    expect(bytes(null)).toBe("—")
  })

  it("affiche les durées", () => {
    const from = new Date("2026-11-01T10:00:00Z")
    expect(duration(from, new Date("2026-11-01T10:00:42Z"))).toBe("42 s")
    expect(duration(from, new Date("2026-11-01T10:03:05Z"))).toBe("3 min 05 s")
    expect(duration(from, new Date("2026-11-01T11:12:00Z"))).toBe("1 h 12 min")
  })

  it("affiche les dates relatives", () => {
    expect(relativeTime(new Date())).toBe("à l'instant")
    expect(relativeTime(new Date(Date.now() - 3 * 60_000))).toBe("il y a 3 minutes")
    expect(relativeTime(null)).toBe("—")
  })
})
