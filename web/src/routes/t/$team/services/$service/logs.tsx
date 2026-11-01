import { createFileRoute } from "@tanstack/react-router"
import { LogsTab } from "@/features/resources/logs"
import { useService } from "@/features/services/use-service"

export const Route = createFileRoute("/t/$team/services/$service/logs")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  const { ref, data, targets } = useService(team, service)
  if (!data) return null
  return <LogsTab resource={ref} targets={targets} param="app" />
}
