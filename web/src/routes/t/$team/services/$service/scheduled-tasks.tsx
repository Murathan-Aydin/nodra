import { createFileRoute } from "@tanstack/react-router"
import { ScheduledTasksTab } from "@/features/resources/scheduled-tasks"
import { useService } from "@/features/services/use-service"

export const Route = createFileRoute("/t/$team/services/$service/scheduled-tasks")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  const { ref, data, targets } = useService(team, service)
  if (!data) return null
  return <ScheduledTasksTab resource={ref} containers={targets} />
}
