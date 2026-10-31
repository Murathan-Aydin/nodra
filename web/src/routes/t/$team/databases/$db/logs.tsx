import { createFileRoute } from "@tanstack/react-router"
import { LogsPanel } from "@/features/resources/logs-panel"
import { dbBase } from "@/features/databases/queries"

export const Route = createFileRoute("/t/$team/databases/$db/logs")({ component: DatabaseLogs })

function DatabaseLogs() {
  const { team, db } = Route.useParams()
  return <LogsPanel team={team} base={dbBase(team, db)} resource={db} />
}
