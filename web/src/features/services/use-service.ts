import { useQuery } from "@tanstack/react-query"
import { serviceQueries, teamQueries } from "@/lib/queries"
import type { ResourceRef } from "@/features/resources/resource"

/** Service courant : détail, résumé (projet et environnement) et référence pour les onglets génériques. */
export function useService(team: string, id: string) {
  const ref: ResourceRef = { team, kind: "service", id }
  const data = useQuery(serviceQueries.service(team, id)).data
  const summary = useQuery(teamQueries.resources(team)).data?.find((r) => r.id === id)
  const apps = data?.apps ?? []
  const targets = apps.map((a) => ({ value: a.name, label: a.name }))
  const back = summary ? `/t/${team}/projects/${summary.project_id}/${summary.environment_id}` : `/t/${team}/projects`
  return { ref, data, summary, apps, targets, back }
}
