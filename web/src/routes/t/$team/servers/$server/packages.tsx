import { createFileRoute } from "@tanstack/react-router"
import { ServerPackages } from "@/features/servers/tools"

export const Route = createFileRoute("/t/$team/servers/$server/packages")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ServerPackages team={team} server={server} />
}
