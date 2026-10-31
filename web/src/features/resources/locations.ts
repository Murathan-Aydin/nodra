import { useQueries, useQuery } from "@tanstack/react-query"
import { get, teamPath } from "@/lib/api"
import type { Destination, Environment, Project, Server } from "@/lib/types"
import { projectsQuery } from "@/features/projects/queries"
import { serversQuery } from "@/features/servers/queries"

/** Tous les environnements visibles de l'équipe, avec leur projet. */
export function useEnvironments(team: string) {
  const projects = useQuery(projectsQuery(team))
  const details = useQueries({
    queries: (projects.data ?? []).map((p) => ({
      queryKey: ["team", team, "project", p.id],
      queryFn: () => get<{ project: Project; environments: Environment[] }>(teamPath(team, `/projects/${p.id}`)),
    })),
  })
  return details.flatMap((d) => (d.data ? d.data.environments.map((e) => ({ ...e, project: d.data.project })) : []))
}

/** Toutes les destinations (réseaux Docker) des serveurs de l'équipe. */
export function useDestinations(team: string) {
  const servers = useQuery(serversQuery(team))
  const details = useQueries({
    queries: (servers.data ?? []).map((s) => ({
      queryKey: ["team", team, "server", s.id],
      queryFn: () => get<{ server: Server; destinations: Destination[] }>(teamPath(team, `/servers/${s.id}`)),
    })),
  })
  return details.flatMap((d) => (d.data ? d.data.destinations.map((x) => ({ ...x, server: d.data.server })) : []))
}
