import { createFileRoute } from "@tanstack/react-router"
import { ScheduledPanel } from "@/features/resources/scheduled-panel"

export const Route = createFileRoute("/t/$team/databases/$db/tasks")({ component: DatabaseTasks })

function DatabaseTasks() {
  const { team, db } = Route.useParams()
  return <ScheduledPanel team={team} resource={db} />
}
