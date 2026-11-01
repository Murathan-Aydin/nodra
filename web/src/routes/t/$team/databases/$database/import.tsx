import { createFileRoute } from "@tanstack/react-router"
import { ImportTab } from "@/features/databases/import"

export const Route = createFileRoute("/t/$team/databases/$database/import")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <ImportTab resource={{ team, kind: "database", id: database }} />
}
