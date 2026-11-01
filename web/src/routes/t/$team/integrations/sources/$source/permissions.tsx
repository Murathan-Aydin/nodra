import { createFileRoute } from "@tanstack/react-router"
import { SourcePermissions } from "@/features/integrations/github-app"

export const Route = createFileRoute("/t/$team/integrations/sources/$source/permissions")({ component: Page })

function Page() {
  const { team, source } = Route.useParams()
  return <SourcePermissions team={team} id={source} />
}
