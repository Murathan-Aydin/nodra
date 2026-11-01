import { createFileRoute } from "@tanstack/react-router"
import { LogsTab } from "@/features/resources/logs"

export const Route = createFileRoute("/t/$team/databases/$database/logs")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <LogsTab resource={{ team, kind: "database", id: database }} />
}
