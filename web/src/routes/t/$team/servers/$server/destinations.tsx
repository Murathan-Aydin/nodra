import { createFileRoute } from "@tanstack/react-router"
import { useServer } from "@/features/servers/server"
import { ServerDestinations } from "@/features/servers/tools"

export const Route = createFileRoute("/t/$team/servers/$server/destinations")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  const { destinations } = useServer(team, server)
  return <ServerDestinations team={team} server={server} destinations={destinations} />
}
