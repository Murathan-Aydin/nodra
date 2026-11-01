import { createFileRoute } from "@tanstack/react-router"
import { useApplication } from "@/features/applications/use-application"
import { TagsTab } from "@/features/resources/tags"

export const Route = createFileRoute("/t/$team/applications/$application/tags")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  const { ref, data } = useApplication(team, application)
  if (!data) return null
  return <TagsTab key={(data.tags ?? []).map((t) => t.id).join()} resource={ref} current={data.tags ?? []} />
}
