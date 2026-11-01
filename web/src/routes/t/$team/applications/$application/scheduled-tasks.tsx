import { createFileRoute } from "@tanstack/react-router"
import { ScheduledTasksTab } from "@/features/resources/scheduled-tasks"

export const Route = createFileRoute("/t/$team/applications/$application/scheduled-tasks")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  return <ScheduledTasksTab resource={{ team, kind: "application", id: application }} />
}
