import { createFileRoute } from "@tanstack/react-router"
import { SourcePage } from "@/features/integrations/github-app"

export const Route = createFileRoute("/t/$team/integrations/sources/$source")({
  validateSearch: (s: Record<string, unknown>): { installed?: string } => (s.installed ? { installed: String(s.installed) } : {}),
  component: Page,
})

function Page() {
  const { team, source } = Route.useParams()
  const { installed } = Route.useSearch()
  return <SourcePage key={source} team={team} id={source} installed={!!installed} />
}
