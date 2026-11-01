import { createFileRoute } from "@tanstack/react-router"
import { ProxyCertificates } from "@/features/servers/proxy"

export const Route = createFileRoute("/t/$team/servers/$server/proxy/certificates")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <ProxyCertificates team={team} server={server} />
}
