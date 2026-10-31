import { describe, expect, it } from "vitest"
import { safeRedirect } from "./utils"
import { bytes, duration, relativeTime } from "./format"

describe("safeRedirect", () => {
  it("n'accepte que les chemins internes", () => {
    expect(safeRedirect("/t/abc/servers?x=1")).toBe("/t/abc/servers?x=1")
    for (const bad of [undefined, "", "https://evil.example", "//evil.example", "/\\evil.example", "javascript:alert(1)"]) {
      expect(safeRedirect(bad)).toBe("/")
    }
  })
})

describe("format", () => {
  it("tailles", () => {
    expect(bytes(512)).toBe("512 o")
    expect(bytes(1536)).toMatch(/^1,5\sKio$/)
  })
  it("durées", () => {
    expect(duration("2026-01-01T00:00:00Z", "2026-01-01T00:01:12Z")).toBe("1 min 12 s")
  })
  it("dates relatives", () => {
    const now = Date.parse("2026-01-01T12:00:00Z")
    expect(relativeTime("2026-01-01T11:57:00Z", now)).toBe("il y a 3 minutes")
    expect(relativeTime(null)).toBe("—")
  })
})
