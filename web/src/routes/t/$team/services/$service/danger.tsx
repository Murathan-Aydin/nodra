import { createFileRoute } from "@tanstack/react-router"
import { DangerTab } from "@/features/resources/danger"
import { useService } from "@/features/services/use-service"

export const Route = createFileRoute("/t/$team/services/$service/danger")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  const { ref, data, back } = useService(team, service)
  if (!data) return null
  return <DangerTab resource={ref} name={data.service.name} back={back} />
}
