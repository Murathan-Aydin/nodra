import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { CheckIcon, PlusIcon } from "lucide-react"
import { toast } from "sonner"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api, get, post } from "@/lib/api"
import type { Tag } from "@/lib/types"
import { cn } from "@/lib/utils"
import { resourceApi, resourceKey, type ResourceRef } from "./resource"

export const tagsQuery = (team: string) => ({ queryKey: ["team", team, "tags"], queryFn: () => get<Tag[]>(`/teams/${team}/tags`) })

/** Pastille d'un tag dans sa couleur. */
export function TagChip({ tag, active, onClick }: { tag: Tag; active?: boolean; onClick?: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn("inline-flex h-7 items-center gap-1.5 rounded-full border px-3 text-xs font-medium transition-colors", active ? "border-primary/60 bg-primary/15 text-foreground" : "border-border bg-muted text-muted-foreground hover:text-foreground")}
    >
      <span className="size-2 rounded-full" style={{ background: tag.color || "var(--muted-foreground)" }} />
      {tag.name}
      {active && <CheckIcon className="size-3" />}
    </button>
  )
}

/** Onglet « Tags » : choix des tags de la ressource (déploiement groupé par tag). */
export function TagsTab({ resource, current }: { resource: ResourceRef; current: Tag[] }) {
  const qc = useQueryClient()
  const all = useQuery(tagsQuery(resource.team)).data ?? []
  const [selected, setSelected] = useState(() => new Set(current.map((t) => t.id)))
  const [name, setName] = useState("")
  const dirty = selected.size !== current.length || current.some((t) => !selected.has(t.id))
  const save = useMutation({
    mutationFn: () => api(resourceApi(resource, "/tags"), { method: "PUT", json: { tag_ids: [...selected] } }),
    onSuccess: () => {
      toast.success("Tags enregistrés")
      void qc.invalidateQueries({ queryKey: resourceKey(resource) })
    },
    onError: (e) => toast.error(e.message),
  })
  const create = useMutation({
    mutationFn: () => post<Tag>(`/teams/${resource.team}/tags`, { name: name.trim() }),
    onSuccess: (t) => {
      setName("")
      setSelected((s) => new Set([...s, t.id]))
      void qc.invalidateQueries({ queryKey: ["team", resource.team, "tags"] })
    },
    onError: (e) => toast.error(e.message),
  })
  const toggle = (id: string) =>
    setSelected((s) => {
      const n = new Set(s)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  return (
    <SettingsCard
      title="Tags"
      description="Les tags permettent de déployer plusieurs ressources d'un coup (POST /deploy?tag=…)."
      actions={
        <Button size="sm" variant="outline" disabled={!dirty} loading={save.isPending} onClick={() => save.mutate()}>
          Enregistrer
        </Button>
      }
    >
      <div className="flex flex-wrap gap-2">
        {all.map((t) => (
          <TagChip key={t.id} tag={t} active={selected.has(t.id)} onClick={() => toggle(t.id)} />
        ))}
        {all.length === 0 && <p className="text-sm text-muted-foreground">Aucun tag dans l'équipe.</p>}
      </div>
      <form
        className="mt-4 flex max-w-sm gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) create.mutate()
        }}
      >
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Nouveau tag" />
        <Button size="sm" variant="outline" type="submit" disabled={!name.trim()} loading={create.isPending}>
          <PlusIcon /> Créer
        </Button>
      </form>
    </SettingsCard>
  )
}
