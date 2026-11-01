import { createFileRoute } from "@tanstack/react-router"
import { ResourceMetricsTab } from "@/features/resources/metrics"

export const Route = createFileRoute("/t/$team/applications/$application/metrics")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  return <ResourceMetricsTab resource={{ team, kind: "application", id: application }} />
}
