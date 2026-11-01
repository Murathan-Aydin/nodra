import { useQuery } from "@tanstack/react-query"
import { applicationQueries, teamQueries } from "@/lib/queries"
import type { ResourceRef } from "@/features/resources/resource"

/** Application courante : détail, résumé (projet et environnement) et référence pour les onglets génériques. */
export function useApplication(team: string, id: string) {
  const ref: ResourceRef = { team, kind: "application", id }
  const data = useQuery(applicationQueries.application(team, id)).data
  const summary = useQuery(teamQueries.resources(team)).data?.find((r) => r.id === id)
  const back = summary ? `/t/${team}/projects/${summary.project_id}/${summary.environment_id}` : `/t/${team}/projects`
  return { ref, data, app: data?.application, back }
}
