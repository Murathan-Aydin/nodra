import { createFileRoute } from "@tanstack/react-router"
import { Dashboard } from "@/features/dashboard/dashboard"

export const Route = createFileRoute("/t/$team/")({ component: DashboardPage })

function DashboardPage() {
  const { team } = Route.useParams()
  return <Dashboard team={team} />
}
