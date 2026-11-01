import { createFileRoute } from "@tanstack/react-router"
import { GroupsPage } from "@/features/team/groups"

export const Route = createFileRoute("/t/$team/team/groups")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <GroupsPage team={team} />
}
