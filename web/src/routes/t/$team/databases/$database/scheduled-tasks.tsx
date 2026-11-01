import { createFileRoute } from "@tanstack/react-router"
import { ScheduledTasksTab } from "@/features/resources/scheduled-tasks"

export const Route = createFileRoute("/t/$team/databases/$database/scheduled-tasks")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <ScheduledTasksTab resource={{ team, kind: "database", id: database }} />
}
