import { useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useDeferredValue, useMemo, useState } from "react"
import { ArrowLeftIcon, BookOpenIcon, FileCode2Icon, SearchIcon } from "lucide-react"
import { CodeEditor } from "@/components/code-editor"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import type { Service, ServiceTemplate } from "@/lib/types"
import { cn } from "@/lib/utils"
import { useDestinations } from "@/features/resources/locations"
import { templateQuery, templatesQuery } from "./queries"

const CUSTOM = "__custom__"

/** Pastille colorée avec l'initiale (les logos du catalogue ne sont pas servis par Nodra). */
export function TemplateMark({ name, className }: { name: string; className?: string }) {
  let h = 0
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) % 360
  return (
    <span
      className={cn("flex size-9 shrink-0 items-center justify-center rounded-lg text-sm font-semibold text-white uppercase", className)}
      style={{ background: `oklch(0.6 0.13 ${h})` }}
      aria-hidden
    >
      {name.slice(0, 1)}
    </span>
  )
}

/** Catalogue des services one-click puis configuration de la création (F10-01, F10-02). */
export function CreateService({ team, environment }: { team: string; environment: string }) {
  const [slug, setSlug] = useState<string | null>(null)
  if (slug) return <ConfigureService team={team} environment={environment} slug={slug} onBack={() => setSlug(null)} />
  return <Catalog onPick={setSlug} />
}

function Catalog({ onPick }: { onPick: (slug: string) => void }) {
  const catalog = useQuery(templatesQuery())
  const [q, setQ] = useState("")
  const [category, setCategory] = useState("all")
  const query = useDeferredValue(q.trim().toLowerCase())
  const list = useMemo(
    () =>
      (catalog.data?.templates ?? []).filter(
        (t) =>
          (category === "all" || t.category === category) &&
          (!query || t.name.toLowerCase().includes(query) || t.slogan.toLowerCase().includes(query) || t.tags?.some((x) => x.toLowerCase().includes(query))),
      ),
    [catalog.data, category, query],
  )
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <div className="relative min-w-60 flex-1">
          <SearchIcon className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input autoFocus className="pl-9" placeholder="Rechercher (nom, usage, tag)…" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Rechercher" />
        </div>
        <Select value={category} onValueChange={setCategory}>
          <SelectTrigger className="w-56" aria-label="Catégorie">
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
        <Button variant="outline" onClick={() => onPick(CUSTOM)}>
          <FileCode2Icon /> Compose personnalisé
        </Button>
      </div>
      {catalog.isPending ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 9 }, (_, i) => (
            <Skeleton key={i} className="h-20" />
          ))}
        </div>
      ) : !list.length ? (
        <EmptyState icon={SearchIcon} title="Aucun service trouvé" />
      ) : (
        <>
          <p className="text-sm text-muted-foreground">{list.length} services</p>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {list.map((t) => (
              <button
                key={t.slug}
                type="button"
                onClick={() => onPick(t.slug)}
                className="flex items-start gap-3 rounded-xl border bg-card p-3 text-left transition-colors hover:border-primary/60 hover:bg-accent/40"
              >
                <TemplateMark name={t.name} />
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">{t.name}</span>
                  <span className="line-clamp-2 text-xs text-muted-foreground">{t.slogan}</span>
                </span>
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}

function ConfigureService({ team, environment, slug, onBack }: { team: string; environment: string; slug: string; onBack: () => void }) {
  const navigate = useNavigate()
  const custom = slug === CUSTOM
  const tpl = useQuery({ ...templateQuery(slug), enabled: !custom })
  const dests = useDestinations(team)
  const [f, setF] = useState({ name: "", description: "", compose: "services:\n  app:\n    image: nginx:alpine\n", destination_id: "" })
  const [startNow, setStartNow] = useState(true)
  const destination = f.destination_id || (dests.length === 1 ? dests[0].id : "")
  const create = useApiMutation(
    () =>
      post<{ service: Service }>(teamPath(team, "/services"), {
        environment_id: environment,
        destination_id: destination,
        ...(custom ? { docker_compose_raw: f.compose } : { template: slug }),
        name: f.name.trim() || undefined,
        description: f.description.trim() || undefined,
        instant_deploy: startNow,
      }),
    {
      success: "Service créé",
      invalidate: [["team", team, "resources"]],
      onSuccess: (r) => void navigate({ to: "/t/$team/services/$svc", params: { team, svc: r.service.id } }),
    },
  )
  const t: ServiceTemplate | undefined = tpl.data
  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate(undefined)
      }}
    >
      <Button type="button" variant="ghost" size="sm" onClick={onBack}>
        <ArrowLeftIcon /> Catalogue
      </Button>
      <Card>
        <CardHeader className="flex-row items-center gap-3">
          {custom ? <FileCode2Icon className="size-9 text-muted-foreground" /> : <TemplateMark name={t?.name ?? slug} />}
          <div className="space-y-1">
            <CardTitle>{custom ? "Compose personnalisé" : (t?.name ?? slug)}</CardTitle>
            <CardDescription>{custom ? "Variables magiques SERVICE_URL_*, SERVICE_PASSWORD_*… reconnues." : t?.slogan}</CardDescription>
          </div>
          {t?.documentation && (
            <Button type="button" variant="outline" size="sm" className="ml-auto" asChild>
              <a href={t.documentation} target="_blank" rel="noreferrer">
                <BookOpenIcon /> Documentation
              </a>
            </Button>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          {t && (
            <div className="flex flex-wrap gap-1">
              <Badge variant="outline">{t.category}</Badge>
              {t.tags?.slice(0, 6).map((x) => (
                <Badge key={x} variant="outline" className="text-muted-foreground">
                  {x}
                </Badge>
              ))}
            </div>
          )}
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Nom" hint="Vide : nom du modèle.">
              <Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder={t?.slug ?? "mon-service"} />
            </Field>
            <Field label="Description">
              <Input value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
            </Field>
          </div>
          {custom ? (
            <Field label="docker-compose.yml">
              <CodeEditor language="yaml" value={f.compose} onChange={(v) => setF({ ...f, compose: v })} minHeight="260px" />
            </Field>
          ) : (
            t?.compose && (
              <details className="rounded-lg border">
                <summary className="cursor-pointer px-3 py-2 text-sm font-medium">docker-compose du modèle (modifiable après la création)</summary>
                <CodeEditor language="yaml" value={t.compose} readOnly minHeight="200px" />
              </details>
            )
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Serveur</CardTitle>
          <CardDescription>Domaines générés sur le domaine joker du serveur (sinon sslip.io) ; secrets générés une fois et visibles dans « Variables ».</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field label="Destination (serveur et réseau Docker)">
            <Select value={destination} onValueChange={(v) => setF({ ...f, destination_id: v })}>
              <SelectTrigger>
                <SelectValue placeholder={dests.length ? "Choisir" : "Aucun serveur : ajoutez-en un"} />
              </SelectTrigger>
              <SelectContent>
                {dests.map((d) => (
                  <SelectItem key={d.id} value={d.id}>
                    {d.server.name} · {d.network}
                    {!d.server.is_usable && " (non validé)"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Label className="font-normal">
            <Checkbox checked={startNow} onCheckedChange={(c) => setStartNow(c === true)} /> Démarrer immédiatement
          </Label>
        </CardContent>
      </Card>
      <div className="flex justify-end">
        <Button type="submit" size="lg" loading={create.isPending} disabled={!destination}>
          Créer le service
        </Button>
      </div>
    </form>
  )
}
