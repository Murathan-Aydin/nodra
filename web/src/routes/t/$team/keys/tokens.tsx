import { createFileRoute } from "@tanstack/react-router"
import { ApiTokensPage } from "@/features/team/keys"

export const Route = createFileRoute("/t/$team/keys/tokens")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <ApiTokensPage team={team} />
}
