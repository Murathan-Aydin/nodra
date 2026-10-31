import { Link, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import { FolderKanbanIcon, LayersIcon, LayoutGridIcon, PlusIcon, ServerIcon, SettingsIcon } from "lucide-react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError, get, post, teamPath } from "@/lib/api"
import { teamQueries } from "@/lib/queries"
import type { Deployment, ProjectSummary, ResourceSummary, Server, ServerMetric } from "@/lib/types"
import { cn } from "@/lib/utils"

const to = (team: string, path: string) => `/t/${team}${path}` as "/"

/** Tableau de bord de l'équipe : projets, serveurs et déploiements récents (disposition de Coolify). */
export function Dashboard({ team }: { team: string }) {
  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_22rem]">
      <div className="min-w-0 space-y-8">
        <Projects team={team} />
        <Servers team={team} />
      </div>
      <RecentDeployments team={team} />
    </div>
  )
}

function SectionTitle({ title, action, children }: { title: string; action?: ReactNode; children?: ReactNode }) {
  return (
    <div className="mb-3 flex items-center justify-between gap-2">
      <h2 className="flex items-center gap-2 text-lg font-semibold tracking-tight [text-shadow:0_1px_8px_rgb(0_0_0/0.25)]">
        {title}
        {action}
      </h2>
      {children}
    </div>
  )
}

function Table({ head, children, cols }: { head: string[]; children: ReactNode; cols: string }) {
  return (
    <div className="glass-panel overflow-hidden rounded-2xl">
      <div className={cn("grid gap-4 border-b border-border px-5 py-3 text-xs font-medium text-muted-foreground", cols)}>
        {head.map((h, i) => (
          <span key={i}>{h}</span>
        ))}
      </div>
      <div className="divide-y divide-border">{children}</div>
    </div>
  )
}

function IconTile({ children }: { children: ReactNode }) {
  return <span className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-primary/25 bg-primary/10 text-primary [&_svg]:size-5">{children}</span>
}

const PROJECT_COLS = "grid-cols-[minmax(0,1.3fr)_7rem_minmax(0,1fr)_4.5rem]"

function Projects({ team }: { team: string }) {
  const projects = useQuery(teamQueries.projects(team))
  const resources = useQuery(teamQueries.resources(team))
  const [creating, setCreating] = useState(false)
  const count = (p: ProjectSummary) => resources.data?.filter((r) => r.project_id === p.id).length ?? 0
  return (
    <section>
      <SectionTitle
        title="Projets"
        action={
          <Button variant="ghost" size="icon-sm" aria-label="Nouveau projet" onClick={() => setCreating(true)}>
            <PlusIcon />
          </Button>
        }
      />
      {projects.isPending ? (
        <Skeleton className="h-40 rounded-2xl" />
      ) : !projects.data?.length ? (
        <Empty icon={<FolderKanbanIcon />} text="Aucun projet pour l'instant." action={<Button onClick={() => setCreating(true)}>Créer un projet</Button>} />
      ) : (
        <Table head={["Projet", "Contenu", "Description", ""]} cols={PROJECT_COLS}>
          {projects.data.map((p) => (
            <div key={p.id} className={cn("grid items-center gap-4 px-5 py-3.5 transition-colors hover:bg-sidebar-accent/50", PROJECT_COLS)}>
              <Link to={to(team, `/projects/${p.id}`)} className="flex min-w-0 items-center gap-3 font-medium">
                <IconTile>
                  <LayoutGridIcon />
                </IconTile>
                <span className="truncate">{p.name}</span>
              </Link>
              <span className="flex items-center gap-4 text-sm text-muted-foreground">
                <span className="flex items-center gap-1.5" title="Environnements">
                  <LayersIcon className="size-4" /> {p.environment_count}
                </span>
                <span className="flex items-center gap-1.5" title="Ressources">
                  <LayoutGridIcon className="size-4" /> {count(p)}
                </span>
              </span>
              <span className="truncate text-sm text-muted-foreground">{p.description || "—"}</span>
              <span className="flex justify-end gap-1">
                <Button variant="ghost" size="icon-sm" className="text-primary" aria-label={`Nouvelle ressource dans ${p.name}`} asChild>
                  <Link to={to(team, `/projects/${p.id}/new`)}>
                    <PlusIcon />
                  </Link>
                </Button>
                <Button variant="ghost" size="icon-sm" className="text-primary" aria-label={`Réglages de ${p.name}`} asChild>
                  <Link to={to(team, `/projects/${p.id}/settings`)}>
                    <SettingsIcon />
                  </Link>
                </Button>
              </span>
            </div>
          ))}
        </Table>
      )}
      <CreateProject team={team} open={creating} onOpenChange={setCreating} />
    </section>
  )
}

function CreateProject({ team, open, onOpenChange }: { team: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [f, setF] = useState({ name: "", description: "" })
  const create = useMutation({
    mutationFn: () => post<ProjectSummary>(teamPath(team, "/projects"), { name: f.name.trim(), description: f.description.trim() }),
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: ["team", team, "projects"] })
      onOpenChange(false)
      setF({ name: "", description: "" })
      toast.success(`Projet « ${p.name} » créé avec son environnement de production`)
      void navigate({ to: to(team, `/projects/${p.id}`) })
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Création impossible"),
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>Nouveau projet</DialogTitle>
            <DialogDescription>Un environnement « production » est créé avec le projet.</DialogDescription>
          </DialogHeader>
          <div className="my-5 space-y-4">
            <Field label="Nom">
              <Input required autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
            </Field>
            <Field label="Description">
              <Input value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
            </Field>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Annuler
            </Button>
            <Button type="submit" loading={create.isPending} disabled={!f.name.trim()}>
              Créer
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

const SERVER_COLS = "grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)_11rem_10rem]"

function Servers({ team }: { team: string }) {
  const servers = useQuery(teamQueries.servers(team))
  return (
    <section>
      <SectionTitle
        title="Serveurs"
        action={
          <Button variant="ghost" size="icon-sm" aria-label="Nouveau serveur" asChild>
            <Link to={to(team, "/servers/new")}>
              <PlusIcon />
            </Link>
          </Button>
        }
      />
      {servers.isPending ? (
        <Skeleton className="h-24 rounded-2xl" />
      ) : !servers.data?.length ? (
        <Empty
          icon={<ServerIcon />}
          text="Aucun serveur connecté."
          action={
            <Button asChild>
              <Link to={to(team, "/servers/new")}>Ajouter un serveur</Link>
            </Button>
          }
        />
      ) : (
        <Table head={["Serveur", "Description", "Utilisation", "Statut"]} cols={SERVER_COLS}>
          {servers.data.map((s) => (
            <div key={s.id} className={cn("grid items-center gap-4 px-5 py-3.5 transition-colors hover:bg-sidebar-accent/50", SERVER_COLS)}>
              <Link to={to(team, `/servers/${s.id}`)} className="flex min-w-0 items-center gap-3 font-medium">
                <IconTile>
                  <ServerIcon />
                </IconTile>
                <span className="truncate">{s.name}</span>
              </Link>
              <span className="truncate text-sm text-muted-foreground">{s.description || (s.is_localhost ? "Serveur hôte de Nodra" : `${s.user}@${s.ip}`)}</span>
              <Usage team={team} server={s} />
              <ServerState server={s} />
            </div>
          ))}
        </Table>
      )}
    </section>
  )
}

function Usage({ team, server }: { team: string; server: Server }) {
  const metrics = useQuery({
    queryKey: ["team", team, "server", server.id, "metrics", "last"],
    queryFn: () => get<ServerMetric[]>(teamPath(team, `/servers/${server.id}/metrics?since=${encodeURIComponent(new Date(Date.now() - 10 * 60_000).toISOString())}`)),
    enabled: server.is_usable,
    refetchInterval: 60_000,
  })
  const last = metrics.data?.at(-1)
  const mem = last && last.mem_total ? (last.mem_used / last.mem_total) * 100 : undefined
  return (
    <div className="space-y-1.5 text-xs text-muted-foreground">
      <Bar label="CPU" value={last?.cpu_percent} />
      <Bar label="Mém." value={mem} />
    </div>
  )
}

function Bar({ label, value }: { label: string; value?: number }) {
  const v = value === undefined ? undefined : Math.max(0, Math.min(100, value))
  return (
    <div className="grid grid-cols-[2.5rem_1fr_2.5rem] items-center gap-2">
      <span>{label}</span>
      <span className="h-1.5 overflow-hidden rounded-full bg-muted">
        {v !== undefined && <span className={cn("block h-full rounded-full", v > 85 ? "bg-destructive" : v > 65 ? "bg-warning" : "bg-primary")} style={{ width: `${v}%` }} />}
      </span>
      <span className="text-right tabular-nums">{v === undefined ? "—" : `${Math.round(v)} %`}</span>
    </div>
  )
}

function ServerState({ server }: { server: Server }) {
  const [label, color] = server.is_usable
    ? ["Opérationnel", "bg-success"]
    : server.validated_at && !server.is_reachable
      ? ["Injoignable", "bg-destructive"]
      : ["Attention requise", "bg-warning"]
  return (
    <span className="flex items-center gap-2 text-sm" title={server.validation_error || undefined}>
      <span className={cn("size-2 shrink-0 rounded-full", color)} />
      {label}
    </span>
  )
}

const DEPLOY_STATUS: Record<Deployment["status"], [string, string]> = {
  finished: ["Succès", "bg-success"],
  failed: ["Échec", "bg-destructive"],
  in_progress: ["En cours", "bg-primary animate-pulse"],
  queued: ["En file", "bg-muted-foreground"],
  cancelled: ["Annulé", "bg-muted-foreground"],
}

function RecentDeployments({ team }: { team: string }) {
  const deployments = useQuery(teamQueries.deployments(team))
  const resources = useQuery(teamQueries.resources(team))
  const servers = useQuery(teamQueries.servers(team))
  const app = (id: string): ResourceSummary | undefined => resources.data?.find((r) => r.id === id)
  const server = (id: string) => servers.data?.find((s) => s.id === id)?.name
  return (
    <section>
      <SectionTitle title="Déploiements récents" />
      <div className="glass-panel overflow-hidden rounded-2xl">
        {!deployments.data?.length ? (
          <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucun déploiement pour l'instant.</p>
        ) : (
          <ul className="divide-y divide-border">
            {deployments.data.map((d) => {
              const a = app(d.application_id)
              const [label, dot] = DEPLOY_STATUS[d.status]
              return (
                <li key={d.id}>
                  <Link to={to(team, `/applications/${d.application_id}/deployments/${d.id}`)} className="block px-5 py-3.5 transition-colors hover:bg-sidebar-accent/50">
                    <p className="truncate font-medium">
                      {a?.name ?? "Application"}
                      {d.pr_number > 0 && <span className="ml-2 text-xs text-muted-foreground">PR #{d.pr_number}</span>}
                    </p>
                    <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      <span className="flex items-center gap-1.5 rounded-full border border-border bg-muted px-2.5 py-1 text-foreground">
                        <span className={cn("size-1.5 rounded-full", dot)} />
                        {label}
                      </span>
                      {a && (
                        <span className="truncate">
                          {a.project_name} / {a.environment_name}
                        </span>
                      )}
                      {server(d.server_id) && <span>· {server(d.server_id)}</span>}
                    </div>
                  </Link>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </section>
  )
}

function Empty({ icon, text, action }: { icon: ReactNode; text: string; action?: ReactNode }) {
  return (
    <div className="glass-panel flex flex-col items-center gap-3 rounded-2xl px-6 py-10 text-center">
      <span className="text-muted-foreground [&_svg]:size-8">{icon}</span>
      <p className="text-sm text-muted-foreground">{text}</p>
      {action}
    </div>
  )
}
