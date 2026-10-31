import type { Permission } from "@/lib/types"

const prefix = (key: string) => key.split(".")[0]

/** Développe les jokers (`*`, `resource.*`) en permissions du catalogue. */
export function expandPermissions(perms: string[], catalog: Permission[]): Set<string> {
  const out = new Set<string>()
  for (const p of catalog) {
    if (perms.includes("*") || perms.includes(p.key) || perms.includes(`${prefix(p.key)}.*`)) out.add(p.key)
  }
  return out
}

/** Recompose la liste envoyée : `*` si tout est coché, `préfixe.*` pour un préfixe complet. */
export function compressPermissions(selected: Set<string>, catalog: Permission[]): string[] {
  if (catalog.length && catalog.every((p) => selected.has(p.key))) return ["*"]
  const out: string[] = []
  for (const pre of [...new Set(catalog.map((p) => prefix(p.key)))]) {
    const keys = catalog.filter((p) => prefix(p.key) === pre).map((p) => p.key)
    if (keys.every((k) => selected.has(k))) out.push(`${pre}.*`)
    else out.push(...keys.filter((k) => selected.has(k)))
  }
  return out
}
