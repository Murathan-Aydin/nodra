import { createFileRoute } from "@tanstack/react-router"
import { SharedVariablesPage } from "@/features/team/variables"

export const Route = createFileRoute("/t/$team/variables/")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <SharedVariablesPage team={team} />
}
