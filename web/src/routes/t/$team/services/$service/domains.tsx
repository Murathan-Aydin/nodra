import { createFileRoute } from "@tanstack/react-router"
import { ServiceDomains } from "@/features/services/service-domains"

export const Route = createFileRoute("/t/$team/services/$service/domains")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  return <ServiceDomains team={team} service={service} />
}
