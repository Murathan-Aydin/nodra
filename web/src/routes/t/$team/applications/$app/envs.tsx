import { createFileRoute } from "@tanstack/react-router"
import { EnvPanel } from "@/features/resources/env-panel"
import { appBase } from "@/features/applications/queries"

export const Route = createFileRoute("/t/$team/applications/$app/envs")({ component: AppEnvs })

function AppEnvs() {
  const { team, app } = Route.useParams()
  return <EnvPanel team={team} base={appBase(team, app)} resource={app} previews />
}
