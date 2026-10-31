import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/storages/")({
  component: () => <ComingSoon title="Stockages S3" card="F11-01" />,
})
