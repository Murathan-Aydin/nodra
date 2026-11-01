import { createFileRoute } from "@tanstack/react-router"
import { ServerTasks } from "@/features/servers/tools"

export const Route = createFileRoute("/t/$team/servers/$server/tasks")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ServerTasks team={team} server={server} />
}
