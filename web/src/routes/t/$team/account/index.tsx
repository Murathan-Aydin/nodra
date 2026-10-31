import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/account/")({
  component: () => <ComingSoon title="Mon compte" card="F2-01" />,
})
