import { createFileRoute } from "@tanstack/react-router"
import { ProxyDynamic } from "@/features/servers/proxy"

export const Route = createFileRoute("/t/$team/servers/$server/proxy/dynamic")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ProxyDynamic team={team} server={server} />
}
