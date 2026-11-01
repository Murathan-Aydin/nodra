import { createFileRoute } from "@tanstack/react-router"
import { useApplication } from "@/features/applications/use-application"
import { OperationsTab } from "@/features/resources/operations"

export const Route = createFileRoute("/t/$team/applications/$application/operations")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  const { ref, app } = useApplication(team, application)
  if (!app) return null
  return <OperationsTab resource={ref} name={app.name} environmentId={app.environment_id} />
}
