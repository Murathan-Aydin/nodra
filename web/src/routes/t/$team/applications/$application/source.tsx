import { createFileRoute } from "@tanstack/react-router"
import { Skeleton } from "@/components/ui/skeleton"
import { useApplication } from "@/features/applications/use-application"
import { ApplicationSource } from "@/features/applications/source"

export const Route = createFileRoute("/t/$team/applications/$application/source")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  const { ref, app } = useApplication(team, application)
  if (!app) return <Skeleton className="h-64 rounded-2xl" />
  return <ApplicationSource key={app.id + String(app.created_at)} resource={ref} app={app} />
}
