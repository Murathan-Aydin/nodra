import { createFileRoute } from "@tanstack/react-router"
import { AccessPanel } from "@/features/team/access-panel"

export const Route = createFileRoute("/t/$team/team/access")({ component: Access })

function Access() {
  const { team } = Route.useParams()
  return <AccessPanel team={team} />
}
