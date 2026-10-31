import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/services/$svc/")({
  component: () => <ComingSoon title="Service" card="F10-03" />,
})
