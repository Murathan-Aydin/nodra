import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/projects/")({
  component: () => <ComingSoon title="Projets" card="F6-01" />,
})
