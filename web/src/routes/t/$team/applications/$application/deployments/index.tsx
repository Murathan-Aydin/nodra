import { createFileRoute } from "@tanstack/react-router"
import { DeploymentsTab } from "@/features/applications/deployments"

export const Route = createFileRoute("/t/$team/applications/$application/deployments/")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  return <DeploymentsTab resource={{ team, kind: "application", id: application }} />
}
