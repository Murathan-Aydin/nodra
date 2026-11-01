import { createFileRoute } from "@tanstack/react-router"
import { HistoryTab } from "@/features/resources/history"

export const Route = createFileRoute("/t/$team/services/$service/tasks")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  return <HistoryTab resource={{ team, kind: "service", id: service }} />
}
