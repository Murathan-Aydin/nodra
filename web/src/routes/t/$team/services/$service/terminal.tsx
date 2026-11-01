import { createFileRoute } from "@tanstack/react-router"
import { resourceApi } from "@/features/resources/resource"
import { TerminalTab } from "@/features/resources/terminal"
import { useService } from "@/features/services/use-service"

export const Route = createFileRoute("/t/$team/services/$service/terminal")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  const { ref, data, targets } = useService(team, service)
  if (!data) return null
  return <TerminalTab path={resourceApi(ref, "/terminal")} targets={targets} param="app" />
}
