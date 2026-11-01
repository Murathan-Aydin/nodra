import { createFileRoute } from "@tanstack/react-router"
import { DeliveriesPage } from "@/features/team/notifications"

export const Route = createFileRoute("/t/$team/notifications/deliveries")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <DeliveriesPage team={team} />
}
