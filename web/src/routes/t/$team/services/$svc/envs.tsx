import { createFileRoute } from "@tanstack/react-router"
import { EnvPanel } from "@/features/resources/env-panel"
import { svcBase } from "@/features/services/queries"

export const Route = createFileRoute("/t/$team/services/$svc/envs")({ component: ServiceEnvs })

function ServiceEnvs() {
  const { team, svc } = Route.useParams()
  return <EnvPanel team={team} base={svcBase(team, svc)} resource={svc} />
}
