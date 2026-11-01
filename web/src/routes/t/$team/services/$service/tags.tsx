import { createFileRoute } from "@tanstack/react-router"
import { TagsTab } from "@/features/resources/tags"
import { useService } from "@/features/services/use-service"

export const Route = createFileRoute("/t/$team/services/$service/tags")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  const { ref, data } = useService(team, service)
  if (!data) return null
  return <TagsTab key={(data.tags ?? []).map((t) => t.id).join()} resource={ref} current={data.tags ?? []} />
}
