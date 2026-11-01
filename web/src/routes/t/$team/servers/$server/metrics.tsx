import { createFileRoute } from "@tanstack/react-router"
import { ServerMetrics } from "@/features/servers/tools"

export const Route = createFileRoute("/t/$team/servers/$server/metrics")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ServerMetrics team={team} server={server} />
}
