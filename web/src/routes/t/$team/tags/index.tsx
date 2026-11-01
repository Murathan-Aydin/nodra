import { createFileRoute } from "@tanstack/react-router"
import { TagsPage } from "@/features/team/tags"

export const Route = createFileRoute("/t/$team/tags/")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <TagsPage team={team} />
}
