import { queryOptions } from "@tanstack/react-query"
import { get } from "./api"
import type { Deployment, Environment, Instance, ProjectSummary, ResourceSummary, Server, Team, User } from "./types"

export const instanceQuery = queryOptions({ queryKey: ["instance"], queryFn: () => get<Instance>("/instance") })

export const meQuery = queryOptions({
  queryKey: ["me"],
  queryFn: () => get<{ user: User; two_factor_enabled: boolean }>("/me"),
  retry: false,
})

export const teamsQuery = queryOptions({ queryKey: ["teams"], queryFn: () => get<Team[]>("/teams") })

export const teamQueries = {
  projects: (team: string) => queryOptions({ queryKey: ["team", team, "projects"], queryFn: () => get<ProjectSummary[]>(`/teams/${team}/projects`) }),
  servers: (team: string) => queryOptions({ queryKey: ["team", team, "servers"], queryFn: () => get<Server[]>(`/teams/${team}/servers`) }),
  resources: (team: string) => queryOptions({ queryKey: ["team", team, "resources"], queryFn: () => get<ResourceSummary[]>(`/teams/${team}/resources`) }),
  deployments: (team: string) =>
    queryOptions({ queryKey: ["team", team, "deployments"], queryFn: () => get<Deployment[]>(`/teams/${team}/deployments?limit=8`), refetchInterval: 15_000 }),
}

export const projectQueries = {
  project: (team: string, project: string) =>
    queryOptions({
      queryKey: ["team", team, "project", project],
      queryFn: () => get<{ project: ProjectSummary; environments: Environment[] }>(`/teams/${team}/projects/${project}`),
    }),
  environment: (team: string, project: string, env: string) =>
    queryOptions({
      queryKey: ["team", team, "project", project, "env", env],
      queryFn: () => get<{ project: ProjectSummary; environment: Environment }>(`/teams/${team}/projects/${project}/environments/${env}`),
    }),
  resources: (team: string, env: string) =>
    queryOptions({
      queryKey: ["team", team, "resources", { env }],
      queryFn: () => get<ResourceSummary[]>(`/teams/${team}/resources?environment_id=${env}`),
      refetchInterval: 15_000,
    }),
}
