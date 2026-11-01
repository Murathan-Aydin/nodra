import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { ExternalLinkIcon, PlayIcon, PlusIcon, SquareIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api, get, post } from "@/lib/api"
import { relativeTime } from "@/lib/format"
import type { Application, Preview } from "@/lib/types"
import { useDraft, useResourcePatch } from "@/features/resources/use-patch"
import { resourceApi, resourceKey, type ResourceRef } from "@/features/resources/resource"
import { SaveButton } from "./general"

/** Onglet « Previews » (Preview Deployments du fork) : une instance par pull request. */
export function PreviewsTab({ resource, app }: { resource: ResourceRef; app: Application }) {
  const key = resourceKey(resource, "previews")
  const previews = useQuery({ queryKey: key, queryFn: () => get<Preview[]>(resourceApi(resource, "/previews")), refetchInterval: 15_000 })
  const [adding, setAdding] = useState(false)
  return (
    <div className="space-y-6">
      <Template resource={resource} app={app} />
      <SettingsCard
        title="Previews"
        description={app.settings.preview_deployments ? "Créées et mises à jour automatiquement par les webhooks des pull requests." : "Les previews automatiques sont désactivées (onglet Avancé) ; vous pouvez en créer à la main."}
        actions={
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            <PlusIcon /> Preview manuelle
          </Button>
        }
        bodyClassName="p-0"
      >
        {adding && <NewPreview resource={resource} queryKey={key} onDone={() => setAdding(false)} />}
        {previews.data?.length
          ? previews.data.map((p) => <PreviewRow key={p.id} resource={resource} preview={p} queryKey={key} />)
          : !adding && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune preview.</p>}
      </SettingsCard>
    </div>
  )
}

function Template({ resource, app }: { resource: ResourceRef; app: Application }) {
  const { draft: d, set, changed, dirty } = useDraft({ preview_url_template: app.preview_url_template })
  const save = useResourcePatch(resource)
  return (
    <SettingsCard title="Modèle d'URL" actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(changed)} />}>
      <Field label="Modèle" hint="Variables : {{pr_id}}, {{domain}} (domaine de l'application), {{random}}.">
        <Input value={d.preview_url_template} onChange={(e) => set("preview_url_template", e.target.value)} className="font-mono" />
      </Field>
    </SettingsCard>
  )
}

function NewPreview({ resource, queryKey, onDone }: { resource: ResourceRef; queryKey: unknown[]; onDone: () => void }) {
  const qc = useQueryClient()
  const [d, setD] = useState({ pr_number: "", title: "", pr_url: "" })
  const create = useMutation({
    mutationFn: () => post(resourceApi(resource, "/previews"), { pr_number: Number(d.pr_number), title: d.title || undefined, pr_url: d.pr_url || undefined }),
    onSuccess: () => {
      toast.success("Preview créée, déploiement lancé")
      void qc.invalidateQueries({ queryKey })
      onDone()
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <form
      className="grid gap-4 border-b border-border p-5 sm:grid-cols-[8rem_1fr_1fr_auto] sm:items-end"
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate()
      }}
    >
      <Field label="N° de PR">
        <Input value={d.pr_number} onChange={(e) => setD({ ...d, pr_number: e.target.value.replace(/\D/g, "") })} required />
      </Field>
      <Field label="Titre">
        <Input value={d.title} onChange={(e) => setD({ ...d, title: e.target.value })} />
      </Field>
      <Field label="Lien de la PR">
        <Input value={d.pr_url} onChange={(e) => setD({ ...d, pr_url: e.target.value })} />
      </Field>
      <div className="flex gap-2">
        <Button type="submit" size="sm" loading={create.isPending}>
          Créer
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDone}>
          Annuler
        </Button>
      </div>
    </form>
  )
}

function PreviewRow({ resource, preview: p, queryKey }: { resource: ResourceRef; preview: Preview; queryKey: unknown[] }) {
  const qc = useQueryClient()
  const [deleting, setDeleting] = useState(false)
  const act = useMutation({
    mutationFn: (a: "deploy" | "stop" | "delete") => (a === "delete" ? api(resourceApi(resource, `/previews/${p.pr_number}`), { method: "DELETE" }) : post(resourceApi(resource, `/previews/${p.pr_number}/${a}`))),
    onSuccess: (_, a) => {
      toast.success(a === "deploy" ? "Déploiement lancé" : a === "stop" ? "Arrêt lancé" : "Preview supprimée")
      setDeleting(false)
      void qc.invalidateQueries({ queryKey })
    },
    onError: (e) => toast.error(e.message),
  })
  const domain = p.domains?.[0]
  return (
    <div className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">
          #{p.pr_number} {p.title}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {p.last_commit && `${p.last_commit.slice(0, 7)} · `}
          {relativeTime(p.created_at)}
          {p.pr_url && (
            <a href={p.pr_url} target="_blank" rel="noreferrer" className="ml-2 underline-offset-2 hover:underline">
              Voir la PR
            </a>
          )}
        </p>
      </div>
      <Pill label={p.status === "running" ? "En cours" : "Arrêtée"} dot={p.status === "running" ? "bg-success" : "bg-muted-foreground"} />
      {domain && (
        <Button size="sm" variant="ghost" asChild>
          <a href={/^https?:/.test(domain) ? domain : `https://${domain}`} target="_blank" rel="noreferrer">
            <ExternalLinkIcon /> Ouvrir
          </a>
        </Button>
      )}
      <Button size="sm" variant="ghost" onClick={() => act.mutate("deploy")}>
        <PlayIcon /> Déployer
      </Button>
      {p.status === "running" && (
        <Button size="sm" variant="ghost" onClick={() => act.mutate("stop")}>
          <SquareIcon /> Arrêter
        </Button>
      )}
      <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
        <Trash2Icon />
      </Button>
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer la preview #${p.pr_number} ?`} actionLabel="Supprimer" loading={act.isPending} onConfirm={() => act.mutate("delete")} />
    </div>
  )
}
