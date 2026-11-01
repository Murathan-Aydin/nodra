import { createFileRoute } from "@tanstack/react-router"
import { ProxyConfig } from "@/features/servers/proxy"

export const Route = createFileRoute("/t/$team/servers/$server/proxy/")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ProxyConfig team={team} server={server} />
}
