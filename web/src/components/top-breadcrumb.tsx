import { useNavigate, useParams, useRouterState } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useRef, useState, type ReactNode } from "react"
import { ChevronsUpDownIcon, SearchIcon } from "lucide-react"
import { NAV } from "@/components/nav"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { projectQueries, teamQueries, teamsQuery } from "@/lib/queries"
import { resourceStatus } from "@/lib/status"
import type { ResourceSummary } from "@/lib/types"
import { cn } from "@/lib/utils"

// Fil d'Ariane de la barre du haut, repris de Coolify (resources/views/components/top-breadcrumb.blade.php) :
// équipe / projet ⇅ / environnement ⇅ / ressource ⇅ + statut, déduits de l'URL.
// Hors projet, le nom de la section (« Projets », « Serveurs »…) remplace le projet.

const RESOURCE_PATH: Record<ResourceSummary["type"], string> = {
  application: "applications",
  database: "databases",
  service: "services",
}

/** Lien vers la page d'une ressource (même forme que le tableau de l'environnement). */
export function resourceHref(team: string, r: ResourceSummary): string {
  return `/t/${team}/${RESOURCE_PATH[r.type]}/${r.id}`
}

/** Nom de la section courante, d'après la navigation latérale. */
function sectionLabel(pathname: string, team: string): string {
  const rest = pathname.slice(`/t/${team}`.length).replace(/\/$/, "")
  for (const g of NAV) for (const i of g.items) if (i.path && (rest === i.path || rest.startsWith(i.path + "/"))) return i.label
  return rest === "" ? "Tableau de bord" : ""
}

export function TopBreadcrumb({ team }: { team: string }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const params = useParams({ strict: false }) as Record<string, string | undefined>
  const resourceId = params.application ?? params.database ?? params.service

  const teams = useQuery(teamsQuery).data ?? []
  const resources = useQuery(teamQueries.resources(team)).data ?? []
  const currentResource = resourceId ? resources.find((r) => r.id === resourceId) : undefined
  const projectId = params.project ?? currentResource?.project_id
  const envId = params.env ?? currentResource?.environment_id

  const projects = useQuery({ ...teamQueries.projects(team), enabled: !!projectId }).data ?? []
  const project = useQuery({ ...projectQueries.project(team, projectId ?? ""), enabled: !!projectId }).data
  const currentProject = projects.find((p) => p.id === projectId) ?? project?.project
  const environments = project?.environments ?? []
  const currentEnvironment = environments.find((e) => e.id === envId)
  const envResources = resources.filter((r) => r.environment_id === envId).sort((a, b) => a.name.localeCompare(b.name))

  const servers = useQuery({ ...teamQueries.servers(team), enabled: !!params.server }).data ?? []
  const currentServer = servers.find((sv) => sv.id === params.server)
  const section = !projectId ? sectionLabel(pathname, team) : ""
  const currentTeam = teams.find((t) => t.id === team)

  return (
    <div className="flex min-w-0 items-center gap-0.5 text-[13px]">
      {/* Équipe */}
      <BreadcrumbSwitcher
        title="Équipes"
        label={currentTeam?.name ?? "Équipe"}
        badge={currentTeam?.name.slice(0, 1) ?? "·"}
        first
        items={teams.map((t) => ({ label: t.name, href: `/t/${t.id}`, active: t.id === team }))}
      />

      {section && (
        <>
          <Separator />
          <span className="flex h-8 min-w-0 items-center truncate px-2 font-semibold opacity-70">{section}</span>
        </>
      )}

      {currentServer && (
        <BreadcrumbSwitcher
          title="Serveurs"
          label={currentServer.name}
          items={servers.map((sv) => ({ label: sv.name, href: `/t/${team}/servers/${sv.id}`, active: sv.id === currentServer.id }))}
        />
      )}

      {currentProject && (
        <BreadcrumbSwitcher
          title="Projets"
          label={currentProject.name}
          items={projects.map((p) => ({ label: p.name, href: `/t/${team}/projects/${p.id}`, active: p.id === currentProject.id }))}
        />
      )}

      {currentProject && currentEnvironment && (
        <BreadcrumbSwitcher
          title="Environnements"
          label={currentEnvironment.name}
          items={environments.map((e) => ({ label: e.name, href: `/t/${team}/projects/${currentProject.id}/${e.id}`, active: e.id === currentEnvironment.id }))}
        />
      )}

      {currentResource && (
        <BreadcrumbSwitcher
          title="Ressources"
          label={currentResource.name}
          searchable
          items={envResources.map((r) => ({ label: r.name, href: resourceHref(team, r), active: r.id === currentResource.id }))}
          meta={<StatusPill status={currentResource.status} />}
        />
      )}
    </div>
  )
}

function Separator() {
  return <span className="shrink-0 px-0.5 text-muted-foreground/50">/</span>
}

type Item = { label: string; href: string; active: boolean }

/** Élément du fil d'Ariane avec menu de changement (x-breadcrumb-switcher de Coolify). */
function BreadcrumbSwitcher({
  title,
  label,
  items,
  meta,
  badge,
  first,
  searchable,
}: {
  title: string
  label: string
  items: Item[]
  meta?: ReactNode
  badge?: string
  first?: boolean
  searchable?: boolean
}) {
  const navigate = useNavigate()
  const [search, setSearch] = useState("")
  const input = useRef<HTMLInputElement>(null)
  const shown = items.filter((i) => i.label.toLowerCase().includes(search.toLowerCase()))
  return (
    <>
      {!first && <Separator />}
      <div className="flex h-8 min-w-0 items-center gap-1 max-lg:shrink-0">
        <DropdownMenu
          onOpenChange={(open) => {
            if (!open) return
            setSearch("")
            // Après la mise au point du menu par Radix, donner le focus au champ de recherche.
            setTimeout(() => input.current?.focus(), 0)
          }}
        >
          <DropdownMenuTrigger
            title={`Changer : ${title.toLowerCase()}`}
            className="flex h-8 min-w-0 items-center gap-1.5 rounded-md px-2 opacity-70 transition-[background-color,opacity] outline-none hover:bg-foreground/5 hover:opacity-100 data-[state=open]:bg-foreground/5 data-[state=open]:opacity-100"
          >
            {badge && <span className="flex size-5 shrink-0 items-center justify-center rounded bg-muted text-[11px] font-semibold uppercase">{badge}</span>}
            <span className="max-w-48 min-w-0 truncate font-semibold">{label}</span>
            <ChevronsUpDownIcon className="size-4 shrink-0 text-muted-foreground" strokeWidth={1.6} />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="max-h-80 w-64 overflow-y-auto">
            {searchable && (
              <div className="relative mb-1">
                <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
                <input
                  ref={input}
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  onKeyDown={(e) => e.key !== "Escape" && e.stopPropagation()}
                  placeholder={`Rechercher (${title.toLowerCase()})`}
                  className="h-8 w-full rounded-md border border-border bg-transparent pr-2 pl-8 text-sm outline-none placeholder:text-muted-foreground"
                />
              </div>
            )}
            <DropdownMenuLabel className="text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">{title}</DropdownMenuLabel>
            {shown.map((i) => (
              <DropdownMenuItem key={i.href} onSelect={() => void navigate({ to: i.href })} className={cn(i.active && "bg-sidebar-accent font-medium")}>
                <span className="min-w-0 flex-1 truncate">{i.label}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        {meta}
      </div>
    </>
  )
}

function StatusPill({ status }: { status: string }) {
  const s = resourceStatus(status)
  return (
    <span className="inline-flex h-[22px] shrink-0 items-center gap-1.5 rounded-full border border-border bg-muted px-2.5 text-xs font-medium">
      <span className={cn("size-1.5 rounded-full", s.dot)} />
      {s.label}
    </span>
  )
}

