import { createFileRoute } from "@tanstack/react-router"
import { PrivateKeysPage } from "@/features/team/keys"

export const Route = createFileRoute("/t/$team/keys/")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <PrivateKeysPage team={team} />
}
