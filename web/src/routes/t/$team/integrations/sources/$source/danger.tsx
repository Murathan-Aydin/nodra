import { createFileRoute } from "@tanstack/react-router"
import { SourceDanger } from "@/features/integrations/github-app"

export const Route = createFileRoute("/t/$team/integrations/sources/$source/danger")({ component: Page })

function Page() {
  const { team, source } = Route.useParams()
  return <SourceDanger team={team} id={source} />
}
