import { createFileRoute } from "@tanstack/react-router"
import { AuditLog } from "@/features/team/audit"

export const Route = createFileRoute("/t/$team/team/audit")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <AuditLog base={`/teams/${team}/audit`} />
}
