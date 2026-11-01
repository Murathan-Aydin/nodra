import { createFileRoute } from "@tanstack/react-router"
import { ResourceWebhooksTab } from "@/features/resources/deploy-webhook"

export const Route = createFileRoute("/t/$team/databases/$database/webhooks")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <ResourceWebhooksTab resource={{ team, kind: "database", id: database }} />
}
