import { createFileRoute, Link, Outlet } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { PageHeader } from "@/components/page-header"
import { TAB_ACTIVE, TAB_LINK, TabNav } from "@/components/tab-nav"
import { Badge } from "@/components/ui/badge"
import { teamQuery } from "@/lib/queries"

export const Route = createFileRoute("/t/$team/team")({ component: TeamLayout })

const TABS = [
  { to: "/t/$team/team", label: "Membres", exact: true },
  { to: "/t/$team/team/groups", label: "Groupes" },
  { to: "/t/$team/team/roles", label: "Rôles" },
  { to: "/t/$team/team/access", label: "Attributions" },
  { to: "/t/$team/team/settings", label: "Paramètres" },
] as const

function TeamLayout() {
  const { team } = Route.useParams()
  const access = useQuery(teamQuery(team)).data
  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            {access?.team.name ?? "Équipe"}
            {access?.team.personal && <Badge variant="outline">Personnelle</Badge>}
            {access?.team.parent_id && <Badge variant="outline">Sous-équipe</Badge>}
          </span>
        }
        description="Membres, groupes, rôles personnalisés et droits par portée."
      />
      <TabNav>
        {TABS.map((t) => (
          <Link key={t.to} to={t.to} params={{ team }} activeOptions={{ exact: "exact" in t }} className={TAB_LINK} activeProps={TAB_ACTIVE}>
            {t.label}
          </Link>
        ))}
      </TabNav>
      <Outlet />
    </>
  )
}
