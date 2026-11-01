import { createFileRoute } from "@tanstack/react-router"
import { Skeleton } from "@/components/ui/skeleton"
import { DatabaseGeneral } from "@/features/databases/database-general"
import { useDatabase } from "@/features/databases/use-database"

export const Route = createFileRoute("/t/$team/databases/$database/")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  const { ref, db } = useDatabase(team, database)
  if (!db) return <Skeleton className="h-64 rounded-2xl" />
  return <DatabaseGeneral key={db.id + db.created_at} resource={ref} db={db} />
}
