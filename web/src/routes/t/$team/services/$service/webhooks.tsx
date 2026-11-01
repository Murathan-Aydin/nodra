import { createFileRoute } from "@tanstack/react-router"
import { ResourceWebhooksTab } from "@/features/resources/deploy-webhook"

export const Route = createFileRoute("/t/$team/services/$service/webhooks")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  return <ResourceWebhooksTab resource={{ team, kind: "service", id: service }} />
}
