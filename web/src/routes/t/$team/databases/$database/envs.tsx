import { createFileRoute } from "@tanstack/react-router"
import { EnvVarsTab } from "@/features/resources/env-vars"

export const Route = createFileRoute("/t/$team/databases/$database/envs")({ component: Page })

function Page() {
  const { team, database } = Route.useParams()
  return <EnvVarsTab resource={{ team, kind: "database", id: database }} />
}
