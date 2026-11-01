import { createFileRoute } from "@tanstack/react-router"
import { OperationsTab } from "@/features/resources/operations"
import { useService } from "@/features/services/use-service"

export const Route = createFileRoute("/t/$team/services/$service/operations")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  const { ref, data } = useService(team, service)
  if (!data) return null
  return <OperationsTab resource={ref} name={data.service.name} environmentId={data.service.environment_id} />
}
