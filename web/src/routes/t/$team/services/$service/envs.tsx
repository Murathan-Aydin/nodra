import { createFileRoute } from "@tanstack/react-router"
import { EnvVarsTab } from "@/features/resources/env-vars"

export const Route = createFileRoute("/t/$team/services/$service/envs")({ component: Page })

function Page() {
  const { team, service } = Route.useParams()
  return <EnvVarsTab resource={{ team, kind: "service", id: service }} />
}
