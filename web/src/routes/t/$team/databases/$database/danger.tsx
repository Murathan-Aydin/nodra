import { createFileRoute } from "@tanstack/react-router"
import { useDatabase } from "@/features/databases/use-database"
import { DangerTab } from "@/features/resources/danger"

export const Route = createFileRoute("/t/$team/databases/$database/danger")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  const { ref, db, back } = useDatabase(team, database)
  if (!db) return null
  return <DangerTab resource={ref} name={db.name} back={back} />
}
