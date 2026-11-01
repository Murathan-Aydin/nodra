import { createFileRoute } from "@tanstack/react-router"
import { useDatabase } from "@/features/databases/use-database"
import { TagsTab } from "@/features/resources/tags"

export const Route = createFileRoute("/t/$team/databases/$database/tags")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  const { ref, data } = useDatabase(team, database)
  if (!data) return null
  return <TagsTab key={(data.tags ?? []).map((t) => t.id).join()} resource={ref} current={data.tags ?? []} />
}
