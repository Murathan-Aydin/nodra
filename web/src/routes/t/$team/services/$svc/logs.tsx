import { createFileRoute } from "@tanstack/react-router"
import { LogsPanel } from "@/features/resources/logs-panel"
import { svcBase } from "@/features/services/queries"

export const Route = createFileRoute("/t/$team/services/$svc/logs")({ component: ServiceLogs })

function ServiceLogs() {
  const { team, svc } = Route.useParams()
  return <LogsPanel team={team} base={svcBase(team, svc)} resource={svc} />
}
