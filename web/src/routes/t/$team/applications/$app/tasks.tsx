import { createFileRoute } from "@tanstack/react-router"
import { ScheduledPanel } from "@/features/resources/scheduled-panel"

export const Route = createFileRoute("/t/$team/applications/$app/tasks")({ component: AppTasks })

function AppTasks() {
  const { team, app } = Route.useParams()
  return <ScheduledPanel team={team} resource={app} />
}
