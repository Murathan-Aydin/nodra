import { queryOptions } from "@tanstack/react-query"
import { get } from "./api"
import type { Application, Container, Database, Deployment, EnvVar, Tag, Environment, Instance, ProjectSummary, ResourceSummary, Server, Team, Service, ServiceApp, User } from "./types"

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

export const serviceQueries = {
  service: (team: string, id: string) =>
    queryOptions({
      queryKey: ["team", team, "service", id],
      queryFn: () => get<{ service: Service; apps: ServiceApp[]; tags: Tag[] | null }>(`/teams/${team}/services/${id}`),
    }),
  containers: (team: string, id: string) =>
    queryOptions({
      queryKey: ["team", team, "service", id, "containers"],
      queryFn: () => get<Container[]>(`/teams/${team}/services/${id}/containers`),
      refetchInterval: 15_000,
    }),
  envs: (team: string, id: string) =>
    queryOptions({
      queryKey: ["team", team, "service", id, "envs"],
      queryFn: () => get<EnvVar[]>(`/teams/${team}/services/${id}/envs`),
    }),
}

export const templateQuery = (slug: string) =>
  queryOptions({
    queryKey: ["service-template", slug],
    queryFn: () => get<{ slug: string; name: string; documentation: string }>(`/service-templates/${slug}`),
    enabled: !!slug,
    staleTime: Infinity,
  })

export const databaseQueries = {
  database: (team: string, id: string) =>
    queryOptions({
      queryKey: ["team", team, "database", id],
      queryFn: () => get<{ database: Database; tags: Tag[] | null; server_id: string; project_id: string }>(`/teams/${team}/databases/${id}`),
    }),
}

export const applicationQueries = {
  application: (team: string, id: string) =>
    queryOptions({
      queryKey: ["team", team, "application", id],
      queryFn: () => get<{ application: Application; tags: Tag[] | null; server_id: string; project_id: string }>(`/teams/${team}/applications/${id}`),
    }),
  deployments: (team: string, id: string) =>
    queryOptions({
      queryKey: ["team", team, "application", id, "deployments"],
      queryFn: () => get<Deployment[]>(`/teams/${team}/applications/${id}/deployments`),
    }),
}
