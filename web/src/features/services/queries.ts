import { queryOptions } from "@tanstack/react-query"
import { get, teamPath } from "@/lib/api"
import type { ServerTask, Service, ServiceApp, ServiceTemplate, Tag } from "@/lib/types"

export const svcBase = (team: string, svc: string) => teamPath(team, `/services/${svc}`)

export const svcQuery = (team: string, svc: string) =>
  queryOptions({
    queryKey: ["team", team, "svc", svc],
    queryFn: () => get<{ service: Service; apps: ServiceApp[]; tags: Tag[] }>(svcBase(team, svc)),
  })

export const svcTasksQuery = (team: string, svc: string) =>
  queryOptions({ queryKey: ["team", team, "svc", svc, "tasks"], queryFn: () => get<ServerTask[]>(`${svcBase(team, svc)}/tasks`) })

/** Catalogue complet (sans compose), filtré côté client pour une recherche instantanée. */
export const templatesQuery = () =>
  queryOptions({
    queryKey: ["service-templates"],
    queryFn: () => get<{ templates: ServiceTemplate[]; categories: string[] }>("/service-templates"),
    staleTime: 10 * 60_000,
  })

export const templateQuery = (slug: string) =>
  queryOptions({ queryKey: ["service-template", slug], queryFn: () => get<ServiceTemplate>(`/service-templates/${slug}`), staleTime: 5 * 60_000 })
