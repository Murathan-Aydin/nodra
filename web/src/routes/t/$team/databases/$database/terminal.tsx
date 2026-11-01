import { createFileRoute } from "@tanstack/react-router"
import { resourceApi } from "@/features/resources/resource"
import { TerminalTab } from "@/features/resources/terminal"

export const Route = createFileRoute("/t/$team/databases/$database/terminal")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <TerminalTab path={resourceApi({ team, kind: "database", id: database }, "/terminal")} />
}
