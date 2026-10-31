import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/sources/")({
  component: () => <ComingSoon title="Sources Git" card="F7-01" />,
})
