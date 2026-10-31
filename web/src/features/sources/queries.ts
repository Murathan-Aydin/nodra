import { queryOptions } from "@tanstack/react-query"
import { get, teamPath } from "@/lib/api"
import type { GitSource, Repo } from "@/lib/types"

export const sourcesQuery = (team: string) =>
  queryOptions({ queryKey: ["team", team, "sources"], queryFn: () => get<GitSource[]>(teamPath(team, "/sources")) })

export const sourceQuery = (team: string, id: string) =>
  queryOptions({ queryKey: ["team", team, "source", id], queryFn: () => get<GitSource>(teamPath(team, `/sources/${id}`)) })

export const reposQuery = (team: string, id: string) =>
  queryOptions({
    queryKey: ["team", team, "source", id, "repos"],
    queryFn: () => get<Repo[]>(teamPath(team, `/sources/${id}/repositories`)),
    staleTime: 60_000,
    retry: false,
  })

export const branchesQuery = (team: string, id: string, repo: string) =>
  queryOptions({
    queryKey: ["team", team, "source", id, "branches", repo],
    queryFn: () => get<string[]>(teamPath(team, `/sources/${id}/branches`), { repo }),
    enabled: !!repo,
    staleTime: 60_000,
    retry: false,
  })
