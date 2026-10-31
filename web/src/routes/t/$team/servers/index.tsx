import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/servers/")({
  component: () => <ComingSoon title="Serveurs" card="F5-01" />,
})
