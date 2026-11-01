import { createFileRoute } from "@tanstack/react-router"
import { RolesPage } from "@/features/team/roles"

export const Route = createFileRoute("/t/$team/team/roles")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <RolesPage team={team} />
}
