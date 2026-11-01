import { createFileRoute } from "@tanstack/react-router"
import { TeamDangerPage } from "@/features/team/general"

export const Route = createFileRoute("/t/$team/team/danger")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <TeamDangerPage team={team} />
}
