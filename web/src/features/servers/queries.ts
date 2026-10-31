import { queryOptions } from "@tanstack/react-query"
import { get, teamPath } from "@/lib/api"
import type { Destination, PrivateKey, ResourceSummary, Server, ServerMetric, ServerTask } from "@/lib/types"

export const serversQuery = (team: string) =>
  queryOptions({ queryKey: ["team", team, "servers"], queryFn: () => get<Server[]>(teamPath(team, "/servers")) })

export const serverQuery = (team: string, server: string) =>
  queryOptions({
    queryKey: ["team", team, "server", server],
    queryFn: () => get<{ server: Server; destinations: Destination[] }>(teamPath(team, `/servers/${server}`)),
  })

export const keysQuery = (team: string) =>
  queryOptions({ queryKey: ["team", team, "keys"], queryFn: () => get<PrivateKey[]>(teamPath(team, "/private-keys")) })

export const tasksQuery = (team: string, server: string) =>
  queryOptions({
    queryKey: ["team", team, "server", server, "tasks"],
    queryFn: () => get<ServerTask[]>(teamPath(team, `/servers/${server}/tasks`)),
  })

export const metricsQuery = (team: string, server: string, since: string) =>
  queryOptions({
    queryKey: ["team", team, "server", server, "metrics", since],
    queryFn: () => get<ServerMetric[]>(teamPath(team, `/servers/${server}/metrics`), { since }),
    refetchInterval: 60_000,
  })

export const serverResourcesQuery = (team: string, server: string) =>
  queryOptions({
    queryKey: ["team", team, "server", server, "resources"],
    queryFn: () => get<ResourceSummary[]>(teamPath(team, `/servers/${server}/resources`)),
  })
