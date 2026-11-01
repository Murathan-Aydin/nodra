import { createFileRoute } from "@tanstack/react-router"
import { useApplication } from "@/features/applications/use-application"
import { DangerTab } from "@/features/resources/danger"

export const Route = createFileRoute("/t/$team/applications/$application/danger")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  const { ref, app, back } = useApplication(team, application)
  if (!app) return null
  return <DangerTab resource={ref} name={app.name} back={back} />
}
