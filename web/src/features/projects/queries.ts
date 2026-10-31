import { queryOptions } from "@tanstack/react-query"
import { get, teamPath } from "@/lib/api"
import type { Environment, Project, ResourceSummary, SharedVariable, Tag } from "@/lib/types"

export const projectsQuery = (team: string) =>
  queryOptions({ queryKey: ["team", team, "projects"], queryFn: () => get<Project[]>(teamPath(team, "/projects")) })

export const projectQuery = (team: string, project: string) =>
  queryOptions({
    queryKey: ["team", team, "project", project],
    queryFn: () => get<{ project: Project; environments: Environment[] }>(teamPath(team, `/projects/${project}`)),
  })

export const resourcesQuery = (team: string, filter: { environment_id?: string; type?: string; tag?: string } = {}) =>
  queryOptions({
    queryKey: ["team", team, "resources", filter],
    queryFn: () => get<ResourceSummary[]>(teamPath(team, "/resources"), filter),
  })

export const variablesQuery = (team: string, scope?: { type: string; id: string }) =>
  queryOptions({
    queryKey: ["team", team, "variables", scope?.type ?? "team", scope?.id ?? ""],
    queryFn: () => get<SharedVariable[]>(teamPath(team, "/variables"), { scope_type: scope?.type, scope_id: scope?.id }),
  })

export const tagsQuery = (team: string) => queryOptions({ queryKey: ["team", team, "tags"], queryFn: () => get<Tag[]>(teamPath(team, "/tags")) })
