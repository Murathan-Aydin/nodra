import { createFileRoute } from "@tanstack/react-router"
import { StoragesTab } from "@/features/resources/storages"

export const Route = createFileRoute("/t/$team/databases/$database/storages")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <StoragesTab resource={{ team, kind: "database", id: database }} />
}
