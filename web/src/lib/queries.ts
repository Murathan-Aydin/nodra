// Requêtes TanStack Query partagées (clés stables, réutilisées par les loaders du routeur).
import { queryOptions } from "@tanstack/react-query"
import { get, teamPath } from "./api"
import type { Instance, Me, PermissionCatalog, Team, TeamAccess } from "./types"

export const instanceQuery = queryOptions({
  queryKey: ["instance"],
  queryFn: () => get<Instance>("/instance"),
  staleTime: 60_000,
})

export const meQuery = queryOptions({
  queryKey: ["me"],
  queryFn: () => get<Me>("/me"),
  staleTime: 30_000,
  retry: false,
})

export const teamsQuery = queryOptions({
  queryKey: ["teams"],
  queryFn: () => get<Team[]>("/teams"),
  staleTime: 30_000,
})

export const teamQuery = (team: string) =>
  queryOptions({
    queryKey: ["team", team, "access"],
    queryFn: () => get<TeamAccess>(teamPath(team)),
    staleTime: 30_000,
  })

export const permissionCatalogQuery = queryOptions({
  queryKey: ["permission-catalog"],
  queryFn: () => get<PermissionCatalog>("/permissions"),
  staleTime: Infinity,
})
