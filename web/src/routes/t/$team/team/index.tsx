import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/team/")({
  component: () => <ComingSoon title="Membres et accès" card="F3-03" />,
})
