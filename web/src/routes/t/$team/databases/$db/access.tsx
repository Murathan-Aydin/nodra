import { createFileRoute } from "@tanstack/react-router"
import { AccessPanel } from "@/features/team/access-panel"

export const Route = createFileRoute("/t/$team/databases/$db/access")({ component: DatabaseAccess })

function DatabaseAccess() {
  const { team, db } = Route.useParams()
  return <AccessPanel team={team} scope={{ type: "resource", id: db }} />
}
