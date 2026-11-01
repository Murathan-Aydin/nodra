import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery } from "@tanstack/react-query"
import { useEffect, useRef, useState } from "react"
import { ArrowLeftIcon, BookOpenIcon, SearchIcon, ServerIcon } from "lucide-react"
import { toast } from "sonner"
import { EmptyPanel, PageHeader, Section } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError, get, post, teamPath } from "@/lib/api"
import { teamQueries } from "@/lib/queries"
import type { Server, ServiceTemplate } from "@/lib/types"
import { cn } from "@/lib/utils"
import { AppForm } from "@/features/new-resource/app-forms"
import { ALL_OPTIONS, APPLICATIONS, DATABASES, isDatabase, SERVICE_ICON, type Kind, type Option } from "@/features/new-resource/options"

type Step = "type" | "configure"
type Search = { step?: Step; kind?: string }

export const Route = createFileRoute("/t/$team/projects/$project/$env/new")({
  validateSearch: (s: Record<string, unknown>): Search => ({
    step: s.step === "configure" ? "configure" : undefined,
    kind: typeof s.kind === "string" ? s.kind : undefined,
  }),
  component: NewResource,
})

function NewResource() {
  const { team, env } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const go = (s: Search) => void navigate({ search: (p) => ({ ...p, ...s }) })
  const step = search.step ?? "type"
  const option = ALL_OPTIONS.find((o) => o.id === search.kind)
  // Une équipe = un serveur : la ressource y est créée directement (destination par défaut).
  const servers = useQuery(teamQueries.servers(team))
  const server = servers.data?.[0]
  const service = search.kind?.startsWith("service:") ? search.kind.slice(8) : undefined

  return (
    <>
      <PageHeader
        title={step === "type" ? "Nouvelle ressource" : (option?.name ?? (service ? `Service ${service}` : "Nouvelle ressource"))}
        actions={
          step !== "type" && (
            <Button size="sm" variant="ghost" onClick={() => void navigate({ search: {} })}>
              <ArrowLeftIcon /> Changer de type
            </Button>
          )
        }
      />
      {!server ? (
        servers.isPending ? (
          <Skeleton className="h-32 rounded-2xl" />
        ) : (
          <NoServer team={team} />
        )
      ) : step === "type" ? (
        <>
          <TeamServer server={server} />
          <ChooseType onPick={(kind) => go({ step: "configure", kind })} />
        </>
      ) : (
        search.kind && <Configure team={team} environment={env} kind={search.kind} />
      )}
    </>
  )
}

const FILTERS = [
  { id: "all", label: "Tout" },
  { id: "applications", label: "Applications" },
  { id: "databases", label: "Bases de données" },
  { id: "services", label: "Services" },
]

function ChooseType({ onPick }: { onPick: (kind: string) => void }) {
  const [q, setQ] = useState("")
  const [filter, setFilter] = useState("all")
  const [category, setCategory] = useState("all")
  const catalog = useQuery({
    queryKey: ["service-templates"],
    queryFn: () => get<{ templates: ServiceTemplate[]; categories: string[] }>("/service-templates"),
    staleTime: 10 * 60_000,
  })
  const needle = q.trim().toLowerCase()
  const match = (o: { name: string; description?: string; slogan?: string; tags?: string[] | null }) =>
    !needle || o.name.toLowerCase().includes(needle) || (o.description ?? o.slogan ?? "").toLowerCase().includes(needle) || !!o.tags?.some((t) => t.toLowerCase().includes(needle))
  const apps = APPLICATIONS.filter(match)
  const dbs = DATABASES.filter(match)
  const services = (catalog.data?.templates ?? []).filter((t) => match(t) && (category === "all" || t.category === category))
  const show = (k: string) => filter === "all" || filter === k
  return (
    <div className="space-y-8">
      <div className="glass-panel flex flex-col gap-3 rounded-2xl p-3 sm:flex-row sm:items-center">
        <div className="relative flex-1">
          <SearchIcon className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input autoFocus className="pl-9" placeholder="Rechercher une ressource" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Rechercher une ressource" />
        </div>
        <div className="flex gap-1 rounded-lg bg-muted p-1">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              type="button"
              onClick={() => setFilter(f.id)}
              className={cn("rounded-md px-3 py-1.5 text-xs font-medium", filter === f.id ? "bg-background/60 text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>
      {show("applications") && apps.length > 0 && (
        <Group title="Applications">
          <SubGroup title="Depuis Git" options={apps.filter((a) => a.group === "git")} onPick={onPick} />
          <SubGroup title="Depuis Docker" options={apps.filter((a) => a.group === "docker")} onPick={onPick} />
        </Group>
      )}
      {show("databases") && dbs.length > 0 && (
        <Group title="Bases de données">
          <Grid options={dbs} onPick={onPick} />
        </Group>
      )}
      {show("services") && (
        <Group
          title="Services"
          action={
            <Select value={category} onValueChange={setCategory}>
              <SelectTrigger className="h-8 w-48" aria-label="Catégorie">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Toutes les catégories</SelectItem>
                {catalog.data?.categories.map((c) => (
                  <SelectItem key={c} value={c}>
                    {c}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          }
        >
          {catalog.isPending ? (
            <Skeleton className="h-32 rounded-2xl" />
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {services.slice(0, 120).map((t) => (
                <Tile key={t.slug} icon={<SERVICE_ICON />} tint="text-primary" title={t.name} text={t.slogan} onClick={() => onPick(`service:${t.slug}`)} />
              ))}
              {services.length > 120 && <p className="text-sm text-muted-foreground">… {services.length - 120} autres : affinez la recherche.</p>}
              {!services.length && <p className="text-sm text-muted-foreground">Aucun service ne correspond.</p>}
            </div>
          )}
        </Group>
      )}
    </div>
  )
}

function Group({ title, action, children }: { title: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-lg font-semibold tracking-tight [text-shadow:0_1px_8px_rgb(0_0_0/0.25)]">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}

function SubGroup({ title, options, onPick }: { title: string; options: Option[]; onPick: (k: string) => void }) {
  if (!options.length) return null
  return (
    <div className="space-y-2">
      <p className="text-xs font-medium tracking-wider text-muted-foreground uppercase">{title}</p>
      <Grid options={options} onPick={onPick} />
    </div>
  )
}

function Grid({ options, onPick }: { options: Option[]; onPick: (k: string) => void }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {options.map((o) => (
        <Tile key={o.id} icon={<o.icon />} tint={o.tint} title={o.name} text={o.description} onClick={() => onPick(o.id)} />
      ))}
    </div>
  )
}

function Tile({ icon, tint, title, text, onClick }: { icon: React.ReactNode; tint: string; title: string; text: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="glass-panel group flex items-start gap-3 rounded-2xl p-4 text-left transition-all hover:-translate-y-0.5 hover:border-primary/50">
      <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-xl border border-border bg-muted [&_svg]:size-5", tint)}>{icon}</span>
      <span className="min-w-0">
        <span className="block truncate text-sm font-medium group-hover:text-primary">{title}</span>
        <span className="line-clamp-2 text-xs text-muted-foreground">{text}</span>
      </span>
    </button>
  )
}

function NoServer({ team }: { team: string }) {
  return (
    <EmptyPanel
      icon={<ServerIcon />}
      title="Cette équipe n'a pas encore de serveur"
      text="Chaque équipe dispose d'un serveur : toutes ses ressources y sont déployées."
      action={
        <Button asChild>
          <Link to={`/t/${team}/servers/new` as "/"}>Ajouter le serveur</Link>
        </Button>
      }
    />
  )
}

/** Serveur de l'équipe, sur lequel la ressource sera créée. */
function TeamServer({ server }: { server: Server }) {
  return (
    <div className="glass-panel mb-6 flex flex-wrap items-center gap-3 rounded-2xl px-4 py-3 text-sm">
      <ServerIcon className="size-4 text-muted-foreground" />
      <span>
        Déployé sur <strong>{server.name}</strong>
        <span className="text-muted-foreground"> · {server.is_localhost ? "serveur hôte de Nodra" : server.ip}</span>
      </span>
      {!server.is_usable && <span className="text-warning">Serveur pas encore validé : la ressource sera créée mais ne pourra démarrer qu'après validation.</span>}
    </div>
  )
}

function Configure({ team, environment, kind }: { team: string; environment: string; kind: string }) {
  const navigate = useNavigate()
  const open = (path: string) => void navigate({ to: `/t/${team}/${path}` as "/" })
  if (kind.startsWith("service:")) return <OneClick team={team} kind={kind} environment={environment} onCreated={(id) => open(`services/${id}`)} />
  if (isDatabase(kind)) return <OneClick team={team} kind={kind} environment={environment} onCreated={(id) => open(`databases/${id}`)} />
  return <AppForm kind={kind as Kind} team={team} environment={environment} onCreated={(id) => open(`applications/${id}`)} />
}

/** Bases de données et services : créés directement, comme dans Coolify (démarrage depuis leur page). */
function OneClick({ team, kind, environment, onCreated }: { team: string; kind: string; environment: string; onCreated: (id: string) => void }) {
  const slug = kind.startsWith("service:") ? kind.slice(8) : undefined
  const template = useQuery({ queryKey: ["service-template", slug], queryFn: () => get<ServiceTemplate & { documentation: string }>(`/service-templates/${slug}`), enabled: !!slug })
  const started = useRef(false)
  const create = useMutation({
    mutationFn: async () =>
      slug
        ? (await post<{ service: { id: string } }>(teamPath(team, "/services"), { environment_id: environment, template: slug })).service.id
        : (await post<{ id: string }>(teamPath(team, "/databases"), { environment_id: environment, type: kind, name: `${kind}-${Math.random().toString(36).slice(2, 8)}` })).id,
    onSuccess: onCreated,
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Création impossible"),
  })
  // Création automatique dès l'arrivée sur l'étape pour les bases (identifiants générés) ; une seule fois.
  const { mutate } = create
  useEffect(() => {
    if (slug || started.current) return
    started.current = true
    mutate()
  }, [slug, mutate])
  if (!slug)
    return (
      <Section title="Création de la base">
        <p className="text-sm text-muted-foreground">{create.isError ? "La création a échoué." : "Création en cours, identifiants générés…"}</p>
        {create.isError && (
          <Button className="mt-4" onClick={() => create.mutate()}>
            Réessayer
          </Button>
        )}
      </Section>
    )
  return (
    <Section title={template.data?.name ?? slug} description={template.data?.slogan}>
      <div className="flex flex-wrap items-center gap-3">
        <Button loading={create.isPending} onClick={() => create.mutate()}>
          Créer le service
        </Button>
        {template.data?.documentation && (
          <Button variant="ghost" asChild>
            <a href={template.data.documentation} target="_blank" rel="noreferrer">
              <BookOpenIcon /> Documentation
            </a>
          </Button>
        )}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">Mots de passe et secrets générés à la création ; domaines générés pour les services exposés.</p>
    </Section>
  )
}

