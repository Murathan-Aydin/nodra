import { useQuery } from "@tanstack/react-query"
import type { ReactNode } from "react"
import { get, teamPath } from "./api"
import { teamQuery } from "./queries"

function grants(perms: string[] | undefined, perm: string): boolean {
  if (!perms) return false
  return perms.some((p) => p === "*" || p === perm || (p.endsWith(".*") && perm.startsWith(p.slice(0, -1))))
}

/** Permission de l'appelant sur l'équipe, ou sur une portée précise (projet, environnement, serveur, ressource). */
export function usePermission(team: string, perm: string, scope?: { type: string; id: string }): boolean {
  const teamAccess = useQuery({ ...teamQuery(team), enabled: !scope })
  const scoped = useQuery({
    queryKey: ["team", team, "permissions", scope?.type, scope?.id],
    queryFn: () =>
      get<{ permissions: string[]; is_owner: boolean }>(teamPath(team, "/permissions"), {
        scope_type: scope?.type,
        scope_id: scope?.id,
      }),
    enabled: !!scope,
    staleTime: 30_000,
  })
  const data = scope ? scoped.data : teamAccess.data
  return !!data && (data.is_owner || grants(data.permissions, perm))
}

/** Affiche les enfants seulement si la permission est accordée (le backend refuse de toute façon). */
export function Can({
  team,
  perm,
  scope,
  children,
  fallback = null,
}: {
  team: string
  perm: string
  scope?: { type: string; id: string }
  children: ReactNode
  fallback?: ReactNode
}) {
  return usePermission(team, perm, scope) ? <>{children}</> : <>{fallback}</>
}
