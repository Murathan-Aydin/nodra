import { createFileRoute } from "@tanstack/react-router"
import { resourceApi } from "@/features/resources/resource"
import { TerminalTab } from "@/features/resources/terminal"

export const Route = createFileRoute("/t/$team/applications/$application/terminal")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  return <TerminalTab path={resourceApi({ team, kind: "application", id: application }, "/terminal")} />
}
