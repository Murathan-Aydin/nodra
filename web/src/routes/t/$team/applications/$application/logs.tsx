import { createFileRoute } from "@tanstack/react-router"
import { LogsTab } from "@/features/resources/logs"

export const Route = createFileRoute("/t/$team/applications/$application/logs")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  return <LogsTab resource={{ team, kind: "application", id: application }} />
}
