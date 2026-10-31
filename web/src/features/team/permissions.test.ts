import { describe, expect, it } from "vitest"
import type { Permission } from "@/lib/types"
import { compressPermissions, expandPermissions } from "./permissions"

const catalog: Permission[] = [
  { key: "team.read", group: "Équipe", description: "" },
  { key: "team.update", group: "Équipe", description: "" },
  { key: "resource.read", group: "Ressources", description: "" },
  { key: "resource.deploy", group: "Ressources", description: "" },
]

describe("permissions", () => {
  it("développe les jokers", () => {
    expect([...expandPermissions(["*"], catalog)]).toEqual(catalog.map((p) => p.key))
    expect([...expandPermissions(["resource.*", "team.read"], catalog)].sort()).toEqual(["resource.deploy", "resource.read", "team.read"])
  })
  it("recompose avec des jokers", () => {
    expect(compressPermissions(new Set(catalog.map((p) => p.key)), catalog)).toEqual(["*"])
    expect(compressPermissions(new Set(["resource.read", "resource.deploy", "team.read"]), catalog)).toEqual(["team.read", "resource.*"])
    expect(compressPermissions(new Set(["resource.deploy"]), catalog)).toEqual(["resource.deploy"])
  })
})
