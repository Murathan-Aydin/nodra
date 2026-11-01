import { useQuery } from "@tanstack/react-query"
import { databaseQueries, teamQueries } from "@/lib/queries"
import type { ResourceRef } from "@/features/resources/resource"

/** Base courante : détail, résumé (projet et environnement) et référence pour les onglets génériques. */
export function useDatabase(team: string, id: string) {
  const ref: ResourceRef = { team, kind: "database", id }
  const data = useQuery(databaseQueries.database(team, id)).data
  const summary = useQuery(teamQueries.resources(team)).data?.find((r) => r.id === id)
  const back = summary ? `/t/${team}/projects/${summary.project_id}/${summary.environment_id}` : `/t/${team}/projects`
  return { ref, data, db: data?.database, back }
}
