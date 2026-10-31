import { createFileRoute } from "@tanstack/react-router"
import { EnvPanel } from "@/features/resources/env-panel"
import { dbBase } from "@/features/databases/queries"

export const Route = createFileRoute("/t/$team/databases/$db/envs")({ component: DatabaseEnvs })

function DatabaseEnvs() {
  const { team, db } = Route.useParams()
  return <EnvPanel team={team} base={dbBase(team, db)} resource={db} />
}
