import { createFileRoute } from "@tanstack/react-router"
import { ServerLogDrains } from "@/features/servers/tools"

export const Route = createFileRoute("/t/$team/servers/$server/log-drains")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ServerLogDrains team={team} server={server} />
}
