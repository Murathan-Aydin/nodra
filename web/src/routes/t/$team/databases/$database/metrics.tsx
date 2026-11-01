import { createFileRoute } from "@tanstack/react-router"
import { ResourceMetricsTab } from "@/features/resources/metrics"

export const Route = createFileRoute("/t/$team/databases/$database/metrics")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <ResourceMetricsTab resource={{ team, kind: "database", id: database }} />
}
