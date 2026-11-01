import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { SquareTerminalIcon } from "lucide-react"
import { EmptyPanel, PageHeader } from "@/components/page"
import { TerminalTab } from "@/features/resources/terminal"
import { teamQueries } from "@/lib/queries"

export const Route = createFileRoute("/t/$team/terminal/")({ component: TerminalPage })

/** Terminal (menu principal) : shell sur le serveur de l'équipe, comme la page Terminal du fork. */
function TerminalPage() {
  const { team } = Route.useParams()
  const server = useQuery(teamQueries.servers(team)).data?.[0]
  return (
    <>
      <PageHeader title="Terminal" description={server ? `Shell sur ${server.name} (${server.user}@${server.ip}).` : undefined} />
      {server ? <TerminalTab path={`/teams/${team}/servers/${server.id}/terminal`} /> : <EmptyPanel icon={<SquareTerminalIcon />} title="Aucun serveur" text="Ajoutez d'abord un serveur." />}
    </>
  )
}
