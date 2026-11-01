import { createFileRoute } from "@tanstack/react-router"
import { useDatabase } from "@/features/databases/use-database"
import { OperationsTab } from "@/features/resources/operations"

export const Route = createFileRoute("/t/$team/databases/$database/operations")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  const { ref, db } = useDatabase(team, database)
  if (!db) return null
  return <OperationsTab resource={ref} name={db.name} environmentId={db.environment_id} />
}
