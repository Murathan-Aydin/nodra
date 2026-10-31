import { createFileRoute } from "@tanstack/react-router"
import { ScheduledPanel } from "@/features/resources/scheduled-panel"

export const Route = createFileRoute("/t/$team/services/$svc/tasks")({ component: ServiceTasks })

function ServiceTasks() {
  const { team, svc } = Route.useParams()
  return <ScheduledPanel team={team} resource={svc} />
}
