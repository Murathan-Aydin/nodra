import { createFileRoute } from "@tanstack/react-router"
import { AccessPanel } from "@/features/team/access-panel"

export const Route = createFileRoute("/t/$team/applications/$app/access")({ component: AppAccess })

function AppAccess() {
  const { team, app } = Route.useParams()
  return <AccessPanel team={team} scope={{ type: "resource", id: app }} />
}
