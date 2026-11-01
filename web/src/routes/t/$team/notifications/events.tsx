import { createFileRoute } from "@tanstack/react-router"
import { SubscriptionsPage } from "@/features/team/notifications"

export const Route = createFileRoute("/t/$team/notifications/events")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <SubscriptionsPage team={team} />
}
