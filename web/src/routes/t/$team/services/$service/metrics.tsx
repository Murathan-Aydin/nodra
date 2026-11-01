import { createFileRoute } from "@tanstack/react-router"
import { ResourceMetricsTab } from "@/features/resources/metrics"

export const Route = createFileRoute("/t/$team/services/$service/metrics")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  return <ResourceMetricsTab resource={{ team, kind: "service", id: service }} />
}
