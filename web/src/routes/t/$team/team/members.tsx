import { createFileRoute } from "@tanstack/react-router"
import { MembersPage } from "@/features/team/members"

export const Route = createFileRoute("/t/$team/team/members")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <MembersPage team={team} />
}
