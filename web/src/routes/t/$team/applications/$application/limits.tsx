import { createFileRoute } from "@tanstack/react-router"
import { Skeleton } from "@/components/ui/skeleton"
import { useApplication } from "@/features/applications/use-application"
import { LimitsCard } from "@/features/resources/limits"

export const Route = createFileRoute("/t/$team/applications/$application/limits")({ component: Page })

function Page() {
  const { team, application } = Route.useParams()
  const { ref, app } = useApplication(team, application)
  if (!app) return <Skeleton className="h-64 rounded-2xl" />
  return <LimitsCard key={JSON.stringify(app.limits)} resource={ref} limits={app.limits} />
}
