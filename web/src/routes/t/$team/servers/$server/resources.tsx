import { createFileRoute } from "@tanstack/react-router"
import { ServerResources } from "@/features/servers/tools"

export const Route = createFileRoute("/t/$team/servers/$server/resources")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ServerResources team={team} server={server} />
}
