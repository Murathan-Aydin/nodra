import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/applications/$app/")({
  component: () => <ComingSoon title="Application" card="F8-02" />,
})
