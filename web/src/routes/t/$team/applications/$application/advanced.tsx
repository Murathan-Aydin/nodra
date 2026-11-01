import { createFileRoute } from "@tanstack/react-router"
import { Skeleton } from "@/components/ui/skeleton"
import { useApplication } from "@/features/applications/use-application"
import { ApplicationAdvanced } from "@/features/applications/advanced"

export const Route = createFileRoute("/t/$team/applications/$application/advanced")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  const { ref, app } = useApplication(team, application)
  if (!app) return <Skeleton className="h-64 rounded-2xl" />
  return <ApplicationAdvanced key={app.id + String(app.created_at)} resource={ref} app={app} />
}
