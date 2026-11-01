import { createFileRoute } from "@tanstack/react-router"
import { StoragesTab } from "@/features/resources/storages"

export const Route = createFileRoute("/t/$team/applications/$application/storages")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  return <StoragesTab resource={{ team, kind: "application", id: application }} />
}
