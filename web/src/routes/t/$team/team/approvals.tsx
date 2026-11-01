import { createFileRoute } from "@tanstack/react-router"
import { ApprovalsPage } from "@/features/team/approvals"

export const Route = createFileRoute("/t/$team/team/approvals")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <ApprovalsPage team={team} />
}
