import { createFileRoute } from "@tanstack/react-router"
import { WebhooksTab } from "@/features/applications/webhooks"

export const Route = createFileRoute("/t/$team/applications/$application/webhooks")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  return <WebhooksTab resource={{ team, kind: "application", id: application }} />
}
