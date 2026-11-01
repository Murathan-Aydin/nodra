import { createFileRoute } from "@tanstack/react-router"
import { RegistriesPage } from "@/features/integrations/integrations"

export const Route = createFileRoute("/t/$team/integrations/registries")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <RegistriesPage team={team} />
}
