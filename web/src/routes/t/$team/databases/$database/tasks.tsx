import { createFileRoute } from "@tanstack/react-router"
import { HistoryTab } from "@/features/resources/history"

export const Route = createFileRoute("/t/$team/databases/$database/tasks")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <HistoryTab resource={{ team, kind: "database", id: database }} />
}
