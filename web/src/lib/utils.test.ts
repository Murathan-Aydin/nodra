import { describe, expect, it } from "vitest"
import { safeRedirect } from "./utils"

describe("safeRedirect", () => {
  it("n'accepte que les chemins internes", () => {
    expect(safeRedirect("/t/abc/servers?x=1")).toBe("/t/abc/servers?x=1")
    for (const bad of [undefined, "", "https://evil.example", "//evil.example", "/\\evil.example", "javascript:alert(1)"]) {
      expect(safeRedirect(bad)).toBe("/")
    }
  })
})
