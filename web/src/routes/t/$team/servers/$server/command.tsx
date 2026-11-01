import { createFileRoute } from "@tanstack/react-router"
import { ServerCommand } from "@/features/servers/tools"

export const Route = createFileRoute("/t/$team/servers/$server/command")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ServerCommand team={team} server={server} />
}
