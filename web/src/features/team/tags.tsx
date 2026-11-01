import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useState } from "react"
import { PlusIcon, RocketIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { PageHeader, SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { resourcePage } from "@/features/resources/resource"
import { tagsQuery } from "@/features/resources/tags"
import { api, post } from "@/lib/api"
import { teamQueries } from "@/lib/queries"
import type { ResourceSummary, Tag } from "@/lib/types"

/** Page « Tags » (Tags du fork) : tags de l'équipe, couleur, ressources étiquetées et déploiement groupé. */
export function TagsPage({ team }: { team: string }) {
  const qc = useQueryClient()
  const tags = useQuery(tagsQuery(team))
  const resources = useQuery(teamQueries.resources(team)).data ?? []
  const [name, setName] = useState("")
  const create = useMutation({
    mutationFn: () => post(`/teams/${team}/tags`, { name: name.trim() }),
    onSuccess: () => {
      setName("")
      void qc.invalidateQueries({ queryKey: ["team", team, "tags"] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <>
      <PageHeader title="Tags" description="Regroupez des ressources pour les déployer ensemble (POST /deploy?tag=…)." />
      <form
        className="mb-6 flex max-w-md gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) create.mutate()
        }}
      >
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Nouveau tag" />
        <Button type="submit" size="sm" disabled={!name.trim()} loading={create.isPending}>
          <PlusIcon /> Créer
        </Button>
      </form>
      <div className="space-y-4">
        {tags.data?.map((t) => <TagCard key={t.id + t.name + t.color} team={team} tag={t} resources={resources.filter((r) => (r as ResourceSummary & { tags?: Tag[] }).tags?.some((x) => x.id === t.id))} />)}
        {tags.data?.length === 0 && <p className="text-sm text-muted-foreground">Aucun tag.</p>}
      </div>
    </>
  )
}

function TagCard({ team, tag, resources }: { team: string; tag: Tag; resources: ResourceSummary[] }) {
  const qc = useQueryClient()
  const [name, setName] = useState(tag.name)
  const [color, setColor] = useState(tag.color || "#3b82f6")
  const [deleting, setDeleting] = useState(false)
  const save = useMutation({
    mutationFn: () => api(`/teams/${team}/tags/${tag.id}`, { method: "PATCH", json: { name, color } }),
    onSuccess: () => (toast.success("Tag enregistré"), void qc.invalidateQueries({ queryKey: ["team", team, "tags"] })),
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api(`/teams/${team}/tags/${tag.id}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["team", team, "tags"] }),
    onError: (e) => toast.error(e.message),
  })
  const deploy = useMutation({
    mutationFn: () => post<{ results: { status: number }[] }>(`/teams/${team}/deploy`, { tags: [tag.name] }),
    onSuccess: (r) => toast.success(`${r.results.filter((x) => x.status === 202).length} ressource(s) lancée(s) sur ${r.results.length}`),
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title={tag.name}
      actions={
        <>
          <Button size="sm" variant="outline" loading={deploy.isPending} onClick={() => deploy.mutate()}>
            <RocketIcon /> Tout déployer
          </Button>
          <Button size="sm" variant="outline" disabled={name === tag.name && color === (tag.color || "#3b82f6")} loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
          <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
            <Trash2Icon />
          </Button>
        </>
      }
    >
      <div className="flex flex-wrap items-center gap-3">
        <input type="color" value={color} onChange={(e) => setColor(e.target.value)} className="size-9 cursor-pointer rounded-lg border border-border bg-transparent" aria-label="Couleur" />
        <Input value={name} onChange={(e) => setName(e.target.value)} className="max-w-xs" />
      </div>
      {resources.length > 0 && (
        <div className="mt-4 flex flex-wrap gap-2">
          {resources.map((r) => (
            <Link key={r.id} to={resourcePage({ team, kind: r.type, id: r.id })} className="rounded-full border border-border bg-muted px-3 py-1 text-xs hover:text-foreground">
              {r.name}
            </Link>
          ))}
        </div>
      )}
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer le tag « ${tag.name} » ?`} description="Les ressources perdent ce tag." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </SettingsCard>
  )
}
