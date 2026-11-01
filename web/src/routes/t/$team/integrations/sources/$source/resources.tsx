import { createFileRoute } from "@tanstack/react-router"
import { SourceResources } from "@/features/integrations/github-app"

export const Route = createFileRoute("/t/$team/integrations/sources/$source/resources")({ component: Page })

function Page() {
  const { team, source } = Route.useParams()
  return <SourceResources team={team} id={source} />
}
