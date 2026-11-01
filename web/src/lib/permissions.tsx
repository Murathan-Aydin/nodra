import { useQuery } from "@tanstack/react-query"
import type { ReactNode } from "react"
import { get } from "./api"

export type Scope = { type: "team" | "project" | "environment" | "server" | "resource"; id: string }

export type Catalog = {
  permissions: { key: string; group: string; description: string }[]
  scope_types: string[]
  token_scopes: string[]
  gateable_actions: string[]
}

/** Catalogue des permissions (libellés, catégories, actions soumises à approbation). */
export const catalogQuery = { queryKey: ["permissions"], queryFn: () => get<Catalog>("/permissions"), staleTime: Infinity }

/** Permissions effectives de l'utilisateur sur une portée (équipe par défaut), mises en cache par portée. */
export function usePermissions(team: string, scope?: Scope) {
  const q = scope && scope.type !== "team" ? `?scope_type=${scope.type}&scope_id=${scope.id}` : ""
  return useQuery({
    queryKey: ["team", team, "my-permissions", scope?.type ?? "team", scope?.id ?? team],
    queryFn: async () => {
      const r = await get<string[] | { permissions: string[] }>(`/teams/${team}/permissions${q}`)
      return new Set(Array.isArray(r) ? r : r.permissions)
    },
    staleTime: 60_000,
  })
}

/** Vrai si l'utilisateur a la permission (jokers « * » et « préfixe.* » acceptés). */
export function usePermission(team: string, perm: string, scope?: Scope): boolean {
  const set = usePermissions(team, scope).data
  if (!set) return false
  if (set.has("*") || set.has(perm)) return true
  return [...set].some((p) => p.endsWith(".*") && perm.startsWith(p.slice(0, -1)))
}

/** Affiche ses enfants seulement si la permission est accordée (ex. boutons de déploiement). */
export function Can({ team, perm, scope, children, fallback = null }: { team: string; perm: string; scope?: Scope; children: ReactNode; fallback?: ReactNode }) {
  return usePermission(team, perm, scope) ? children : fallback
}
