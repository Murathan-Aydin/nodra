import { createFileRoute } from "@tanstack/react-router"
import { TerminalPanel } from "@/features/resources/terminal-panel"
import { svcBase } from "@/features/services/queries"

export const Route = createFileRoute("/t/$team/services/$svc/terminal")({ component: ServiceTerminal })

function ServiceTerminal() {
  const { team, svc } = Route.useParams()
  return <TerminalPanel team={team} base={svcBase(team, svc)} resource={svc} />
}
