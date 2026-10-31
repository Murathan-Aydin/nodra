import { createFileRoute } from "@tanstack/react-router"
import { LogsPanel } from "@/features/resources/logs-panel"
import { appBase } from "@/features/applications/queries"

export const Route = createFileRoute("/t/$team/applications/$app/logs")({ component: AppLogs })

function AppLogs() {
  const { team, app } = Route.useParams()
  return <LogsPanel team={team} base={appBase(team, app)} resource={app} />
}
