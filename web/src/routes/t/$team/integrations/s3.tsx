import { createFileRoute } from "@tanstack/react-router"
import { S3Page } from "@/features/integrations/integrations"

export const Route = createFileRoute("/t/$team/integrations/s3")({ component: Page })

function Page() {
  const { team } = Route.useParams()
  return <S3Page team={team} />
}
