import { createFileRoute } from "@tanstack/react-router"
import { TeamGeneralPage } from "@/features/team/general"

export const Route = createFileRoute("/t/$team/team/")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <TeamGeneralPage team={team} />
}
