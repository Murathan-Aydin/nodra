import { createFileRoute } from "@tanstack/react-router"
import { ChannelsPage } from "@/features/team/notifications"

export const Route = createFileRoute("/t/$team/notifications/")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <ChannelsPage team={team} />
}
