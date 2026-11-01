import { createFileRoute } from "@tanstack/react-router"
import { EnvVarsTab } from "@/features/resources/env-vars"

export const Route = createFileRoute("/t/$team/applications/$application/envs")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  return <EnvVarsTab resource={{ team, kind: "application", id: application }} buildtime />
}
