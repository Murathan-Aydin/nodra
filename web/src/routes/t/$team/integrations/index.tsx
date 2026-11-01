import { createFileRoute } from "@tanstack/react-router"
import { SourcesPage } from "@/features/integrations/integrations"

export const Route = createFileRoute("/t/$team/integrations/")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <SourcesPage team={team} />
}
