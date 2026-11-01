import { createFileRoute } from "@tanstack/react-router"
import { SourceGeneral } from "@/features/integrations/github-app"

export const Route = createFileRoute("/t/$team/integrations/sources/$source/")({
  validateSearch: (s: Record<string, unknown>): { installed?: string; previews?: string } => ({
    ...(s.installed ? { installed: String(s.installed) } : {}),
    ...(s.previews !== undefined ? { previews: String(s.previews) } : {}),
  }),
  component: Page,
})

function Page() {
  const { team, source } = Route.useParams()
  const { installed, previews } = Route.useSearch()
  return <SourceGeneral key={source} team={team} id={source} installed={!!installed} previews={previews} />
}
