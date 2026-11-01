import { createFileRoute } from "@tanstack/react-router"
import { BackupsTab } from "@/features/resources/backups"

export const Route = createFileRoute("/t/$team/databases/$database/backups")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <BackupsTab resource={{ team, kind: "database", id: database }} />
}
