import { createFileRoute } from "@tanstack/react-router"
import { TerminalPanel } from "@/features/resources/terminal-panel"
import { appBase } from "@/features/applications/queries"

export const Route = createFileRoute("/t/$team/applications/$app/terminal")({ component: AppTerminal })

function AppTerminal() {
  const { team, app } = Route.useParams()
  return <TerminalPanel team={team} base={appBase(team, app)} resource={app} />
}
