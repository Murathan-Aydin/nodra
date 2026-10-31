import { createFileRoute } from "@tanstack/react-router"
import { TerminalPanel } from "@/features/resources/terminal-panel"
import { dbBase } from "@/features/databases/queries"

export const Route = createFileRoute("/t/$team/databases/$db/terminal")({ component: DatabaseTerminal })

function DatabaseTerminal() {
  const { team, db } = Route.useParams()
  return <TerminalPanel team={team} base={dbBase(team, db)} resource={db} />
}
