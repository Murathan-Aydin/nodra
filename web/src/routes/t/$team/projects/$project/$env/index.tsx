import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ArrowDownUpIcon, BoxIcon, ChevronDownIcon, ChevronUpIcon, DatabaseIcon, FilterIcon, LayersIcon, PlusIcon, SearchIcon, SettingsIcon, VariableIcon } from "lucide-react"
import { EmptyPanel, PageHeader } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { projectQueries, teamQueries } from "@/lib/queries"
import { resourceStatus } from "@/lib/status"
import type { ResourceSummary } from "@/lib/types"
import { cn } from "@/lib/utils"

export const Route = createFileRoute("/t/$team/projects/$project/$env/")({ component: EnvironmentPage })

const TYPE: Record<ResourceSummary["type"], { label: string; icon: typeof BoxIcon; path: string }> = {
  application: { label: "Application", icon: BoxIcon, path: "applications" },
  database: { label: "Base de données", icon: DatabaseIcon, path: "databases" },
  service: { label: "Service", icon: LayersIcon, path: "services" },
}

type SortKey = "name" | "type" | "status"
const SORTS: Record<SortKey, string> = { name: "Nom", type: "Type", status: "Statut" }

const COLS = "grid-cols-[minmax(0,1.6fr)_9rem_10rem_minmax(0,1.3fr)_8rem_7rem]"

/** Ressources d'un environnement (project/resource/index du fork). */
function EnvironmentPage() {
  const { team, project, env } = Route.useParams()
  const data = useQuery(projectQueries.environment(team, project, env)).data
  const resources = useQuery(projectQueries.resources(team, env))
  const servers = useQuery(teamQueries.servers(team)).data ?? []
  const [q, setQ] = useState("")
  const [type, setType] = useState<"all" | ResourceSummary["type"]>("all")
  const [sort, setSort] = useState<{ key: SortKey; asc: boolean }>({ key: "name", asc: true })
  const newPath = { to: "/t/$team/projects/$project/$env/new", params: { team, project, env } } as const
  const all = resources.data ?? []
  const list = all
    .filter((r) => (!q || r.name.toLowerCase().includes(q.toLowerCase())) && (type === "all" || r.type === type))
    .sort((a, b) => {
      const v = sort.key === "name" ? a.name.localeCompare(b.name) : sort.key === "type" ? a.type.localeCompare(b.type) : a.status.localeCompare(b.status)
      return sort.asc ? v : -v
    })
  const toggleSort = (key: SortKey) => setSort((s) => ({ key, asc: s.key === key ? !s.asc : true }))
  const SortIcon = sort.asc ? ChevronUpIcon : ChevronDownIcon

  return (
    <>
      <PageHeader
        title={
          data ? (
            <>
              {data.environment.name}
              <Link to="/t/$team/projects/$project/$env/settings" params={{ team, project, env }} title="Réglages de l'environnement" className="flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-foreground/5 hover:text-foreground">
                <SettingsIcon className="size-4" />
              </Link>
            </>
          ) : (
            <Skeleton className="h-8 w-40" />
          )
        }
        description={
          <>
            {all.length} ressource{all.length > 1 ? "s" : ""} dans{" "}
            <Link to="/t/$team/projects/$project" params={{ team, project }} className="font-semibold text-foreground hover:underline">
              {data?.project.name ?? "…"}
            </Link>
          </>
        }
        actions={
          <>
            <Button variant="outline" asChild>
              <Link to="/t/$team/variables" params={{ team }}>
                <VariableIcon /> Variables partagées
              </Link>
            </Button>
            <Button asChild>
              <Link {...newPath}>
                <PlusIcon /> Nouvelle ressource
              </Link>
            </Button>
          </>
        }
      />
      {resources.isPending ? (
        <Skeleton className="h-40 rounded-2xl" />
      ) : !all.length ? (
        <EmptyPanel
          icon={<BoxIcon />}
          title="Aucune ressource pour l'instant"
          text="Déployez une application depuis Git ou une image, une base de données ou un service prêt à l'emploi."
          action={
            <Button asChild>
              <Link {...newPath}>
                <PlusIcon /> Nouvelle ressource
              </Link>
            </Button>
          }
        />
      ) : (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-2">
            <div className="relative w-full max-w-md">
              <SearchIcon className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input className="pl-9" placeholder="Rechercher une ressource" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Rechercher une ressource" />
            </div>
            <div className="ml-auto flex gap-2">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" className={cn(type !== "all" && "text-primary")}>
                    <FilterIcon /> Filtrer
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-48">
                  <DropdownMenuLabel>Type</DropdownMenuLabel>
                  <DropdownMenuRadioGroup value={type} onValueChange={(v) => setType(v as typeof type)}>
                    <DropdownMenuRadioItem value="all">Tous</DropdownMenuRadioItem>
                    {Object.entries(TYPE).map(([k, t]) => (
                      <DropdownMenuRadioItem key={k} value={k}>
                        {t.label}
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline">
                    <ArrowDownUpIcon /> Trier
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-48">
                  <DropdownMenuLabel>Trier par</DropdownMenuLabel>
                  <DropdownMenuRadioGroup value={sort.key} onValueChange={(v) => setSort({ key: v as SortKey, asc: true })}>
                    {Object.entries(SORTS).map(([k, label]) => (
                      <DropdownMenuRadioItem key={k} value={k}>
                        {label}
                      </DropdownMenuRadioItem>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
          <div className="glass-panel overflow-x-auto rounded-2xl">
            <div className={cn("grid min-w-[56rem] gap-4 border-b border-border px-5 py-3 text-xs font-medium text-muted-foreground", COLS)}>
              {(["name", "type", "status"] as SortKey[]).map((k) => (
                <button key={k} type="button" onClick={() => toggleSort(k)} className={cn("flex items-center gap-1 text-left hover:text-foreground", sort.key === k && "text-foreground")}>
                  {k === "name" ? "Ressource" : SORTS[k]}
                  {sort.key === k && <SortIcon className="size-3.5" />}
                </button>
              ))}
              <span>Domaine</span>
              <span>Serveur</span>
              <span>Tags</span>
            </div>
            <div className="divide-y divide-border">
              {list.map((r) => {
                const t = TYPE[r.type]
                const st = resourceStatus(r.status)
                const domain = r.domains?.[0]
                return (
                  <div key={r.id} className={cn("relative grid min-w-[56rem] items-center gap-4 px-5 py-3.5 text-sm transition-colors hover:bg-foreground/[0.03]", COLS)}>
                    <Link to={`/t/${team}/${t.path}/${r.id}` as "/"} className="flex min-w-0 items-center gap-3 font-semibold after:absolute after:inset-0">
                      <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-muted-foreground">
                        <t.icon className="size-4" />
                      </span>
                      <span className="truncate">{r.name}</span>
                    </Link>
                    <span className="text-muted-foreground">{t.label}</span>
                    <span>
                      <Pill label={st.label} dot={st.dot} />
                    </span>
                    <span className="truncate">
                      {domain ? (
                        <a href={/^https?:/.test(domain) ? domain : `https://${domain}`} target="_blank" rel="noreferrer" className="relative z-10 text-primary hover:underline">
                          {domain.replace(/^https?:\/\//, "")}
                        </a>
                      ) : (
                        <span className="text-muted-foreground">-</span>
                      )}
                    </span>
                    <span className="truncate text-muted-foreground">{servers.find((s) => s.id === r.server_id)?.name ?? "-"}</span>
                    <span className="flex min-w-0 flex-wrap gap-1">
                      {r.tags?.length ? (
                        r.tags.map((tag) => (
                          <span key={tag.id} className="inline-flex items-center gap-1 rounded-full border border-border bg-muted px-2 py-0.5 text-[11px]">
                            <span className="size-1.5 rounded-full" style={{ background: tag.color || "var(--muted-foreground)" }} />
                            {tag.name}
                          </span>
                        ))
                      ) : (
                        <span className="text-muted-foreground">-</span>
                      )}
                    </span>
                  </div>
                )
              })}
              {!list.length && <p className="px-5 py-6 text-center text-sm text-muted-foreground">Aucune ressource ne correspond.</p>}
            </div>
          </div>
        </>
      )}
    </>
  )
}
