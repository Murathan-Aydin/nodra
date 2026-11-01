import { createFileRoute } from "@tanstack/react-router"
import { DeploymentDetail } from "@/features/applications/deployments"

export const Route = createFileRoute("/t/$team/applications/$application/deployments/$deployment")({ component: Page })

function Page() {
  const { team, application, deployment } = Route.useParams()
  return <DeploymentDetail resource={{ team, kind: "application", id: application }} deploymentId={deployment} />
}
