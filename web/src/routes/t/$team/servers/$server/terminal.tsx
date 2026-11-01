import { createFileRoute } from "@tanstack/react-router"
import { TerminalTab } from "@/features/resources/terminal"

export const Route = createFileRoute("/t/$team/servers/$server/terminal")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return <TerminalTab path={`/teams/${team}/servers/${server}/terminal`} />
}
