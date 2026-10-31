import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery } from "@tanstack/react-query"
import { useEffect, useRef, useState } from "react"
import { ArrowLeftIcon, BookOpenIcon, NetworkIcon, SearchIcon, ServerIcon } from "lucide-react"
import { toast } from "sonner"
import { EmptyPanel, PageHeader, Section } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError, get, post, teamPath } from "@/lib/api"
import { projectQueries, teamQueries } from "@/lib/queries"
import type { Destination, Server, ServiceTemplate } from "@/lib/types"
import { cn } from "@/lib/utils"
import { AppForm } from "@/features/new-resource/app-forms"
import { ALL_OPTIONS, APPLICATIONS, DATABASES, isDatabase, SERVICE_ICON, type Kind, type Option } from "@/features/new-resource/options"

type Step = "type" | "server" | "destination" | "configure"
type Search = { step?: Step; kind?: string; server?: string; destination?: string }

export const Route = createFileRoute("/t/$team/projects/$project/$env/new")({
  validateSearch: (s: Record<string, unknown>): Search => ({
    step: ["type", "server", "destination", "configure"].includes(s.step as string) ? (s.step as Step) : undefined,
    kind: typeof s.kind === "string" ? s.kind : undefined,
    server: typeof s.server === "string" ? s.server : undefined,
    destination: typeof s.destination === "string" ? s.destination : undefined,
  }),
  component: NewResource,
})

function NewResource() {
  const { team, project, env } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const go = (s: Search) => void navigate({ search: (p) => ({ ...p, ...s }) })
  const data = useQuery(projectQueries.environment(team, project, env)).data
  const step = search.step ?? "type"
  const option = ALL_OPTIONS.find((o) => o.id === search.kind)
  const service = search.kind?.startsWith("service:") ? search.kind.slice(8) : undefined

  return (
    <>
      <PageHeader
        crumbs={[
          { label: "Projets", to: `/t/${team}/projects` },
          { label: data?.project.name ?? "…", to: `/t/${team}/projects/${project}` },
          { label: data?.environment.name ?? "…", to: `/t/${team}/projects/${project}/${env}` },
          { label: "Nouvelle ressource" },
        ]}
        title={step === "type" ? "Nouvelle ressource" : (option?.name ?? (service ? `Service ${service}` : "Nouvelle ressource"))}
        actions={
          step !== "type" && (
            <Button size="sm" variant="ghost" onClick={() => void navigate({ search: {} })}>
              <ArrowLeftIcon /> Changer de type
            </Button>
          )
        }
      />
      {step === "type" && <ChooseType onPick={(kind) => go({ step: "server", kind })} />}
      {step === "server" && <ChooseServer team={team} onPick={(server) => go({ step: "destination", server })} />}
      {step === "destination" && search.server && (
        <ChooseDestination team={team} server={search.server} onPick={(destination) => go({ step: "configure", destination })} />
      )}
      {step === "configure" && search.destination && search.kind && (
        <Configure team={team} environment={env} destination={search.destination} kind={search.kind} />
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

function ChooseServer({ team, onPick }: { team: string; onPick: (id: string) => void }) {
  const servers = useQuery(teamQueries.servers(team))
  const usable = servers.data?.filter((s) => s.is_usable) ?? []
  return (
    <Section title="Choisir un serveur" description="La machine qui hébergera cette ressource.">
      {servers.isPending ? (
        <Skeleton className="h-20 rounded-xl" />
      ) : !usable.length ? (
        <EmptyPanel
          icon={<ServerIcon />}
          title="Aucun serveur disponible"
          text="Validez un serveur joignable avant de créer cette ressource."
          action={
            <Button asChild>
              <Link to={`/t/${team}/servers/new` as "/"}>Ajouter un serveur</Link>
            </Button>
          }
        />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {usable.map((s: Server) => (
            <Tile key={s.id} icon={<ServerIcon />} tint="text-primary" title={s.name} text={s.description || (s.is_localhost ? "Serveur hôte de Nodra" : s.ip)} onClick={() => onPick(s.id)} />
          ))}
        </div>
      )}
    </Section>
  )
}

function ChooseDestination({ team, server, onPick }: { team: string; server: string; onPick: (id: string) => void }) {
  const data = useQuery({
    queryKey: ["team", team, "server", server, "detail"],
    queryFn: () => get<{ server: Server; destinations: Destination[] }>(teamPath(team, `/servers/${server}`)),
  }).data
  const dests = data?.destinations ?? []
  // Une seule destination : on passe directement à la suite (comme Coolify).
  const only = dests.length === 1 ? dests[0].id : undefined
  useEffect(() => {
    if (only) onPick(only)
  }, [only, onPick])
  if (!data || only) return <Skeleton className="h-20 rounded-2xl" />
  return (
    <Section title="Choisir une destination" description="Les destinations séparent les ressources par réseau Docker. En cas de doute, prenez celle par défaut.">
      <div className="grid gap-3 sm:grid-cols-2">
        {dests.map((d) => (
          <Tile key={d.id} icon={<NetworkIcon />} tint="text-primary" title={d.name} text={`Réseau ${d.network}${d.is_default ? " · par défaut" : ""}`} onClick={() => onPick(d.id)} />
        ))}
      </div>
    </Section>
  )
}

function Configure({ team, environment, destination, kind }: { team: string; environment: string; destination: string; kind: string }) {
  const navigate = useNavigate()
  const open = (path: string) => void navigate({ to: `/t/${team}/${path}` as "/" })
  if (kind.startsWith("service:")) return <OneClick team={team} kind={kind} environment={environment} destination={destination} onCreated={(id) => open(`services/${id}`)} />
  if (isDatabase(kind)) return <OneClick team={team} kind={kind} environment={environment} destination={destination} onCreated={(id) => open(`databases/${id}`)} />
  return <AppForm kind={kind as Kind} team={team} environment={environment} destination={destination} onCreated={(id) => open(`applications/${id}`)} />
}

/** Bases de données et services : créés directement, comme dans Coolify (démarrage depuis leur page). */
function OneClick({ team, kind, environment, destination, onCreated }: { team: string; kind: string; environment: string; destination: string; onCreated: (id: string) => void }) {
  const slug = kind.startsWith("service:") ? kind.slice(8) : undefined
  const template = useQuery({ queryKey: ["service-template", slug], queryFn: () => get<ServiceTemplate & { documentation: string }>(`/service-templates/${slug}`), enabled: !!slug })
  const started = useRef(false)
  const create = useMutation({
    mutationFn: async () =>
      slug
        ? (await post<{ service: { id: string } }>(teamPath(team, "/services"), { environment_id: environment, destination_id: destination, template: slug })).service.id
        : (await post<{ id: string }>(teamPath(team, "/databases"), { environment_id: environment, destination_id: destination, type: kind, name: `${kind}-${Math.random().toString(36).slice(2, 8)}` })).id,
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

