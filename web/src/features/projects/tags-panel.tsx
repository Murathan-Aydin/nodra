import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { PlusIcon, TagIcon, Trash2Icon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { del, patch, post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { tagsQuery } from "./queries"

const COLORS = ["#6366f1", "#10b981", "#f59e0b", "#ef4444", "#0ea5e9", "#a855f7", "#64748b"]

export function TagChip({ name, color }: { name: string; color?: string }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs">
      <span className="size-2 rounded-full" style={{ background: color || "var(--muted-foreground)" }} />
      {name}
    </span>
  )
}

export function TagsPanel({ team }: { team: string }) {
  const tags = useQuery(tagsQuery(team))
  const canManage = usePermission(team, "resource.update")
  const [name, setName] = useState("")
  const [color, setColor] = useState(COLORS[0])
  const key = tagsQuery(team).queryKey
  const create = useApiMutation(() => post(teamPath(team, "/tags"), { name: name.trim(), color }), {
    success: "Tag créé",
    invalidate: [key],
    onSuccess: () => setName(""),
  })
  const recolor = useApiMutation((v: { id: string; name: string; color: string }) => patch(teamPath(team, `/tags/${v.id}`), { name: v.name, color: v.color }), {
    invalidate: [key],
  })
  const remove = useApiMutation((id: string) => del(teamPath(team, `/tags/${id}`)), { success: "Tag supprimé", invalidate: [key, ["team", team, "resources"]] })
  return (
    <div className="space-y-4">
      {canManage && (
        <form
          className="flex flex-wrap items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (name.trim()) create.mutate(undefined)
          }}
        >
          <Input className="max-w-xs" placeholder="Nom du tag" value={name} onChange={(e) => setName(e.target.value)} />
          <div className="flex gap-1">
            {COLORS.map((c) => (
              <button
                type="button"
                key={c}
                aria-label={`Couleur ${c}`}
                onClick={() => setColor(c)}
                className={`size-6 rounded-full border-2 ${c === color ? "border-foreground" : "border-transparent"}`}
                style={{ background: c }}
              />
            ))}
          </div>
          <Button type="submit" variant="outline" loading={create.isPending}>
            <PlusIcon /> Créer
          </Button>
        </form>
      )}
      {!tags.data?.length ? (
        <EmptyState icon={TagIcon} title="Aucun tag" description="Les tags regroupent des ressources (filtres, déploiements groupés par tag)." />
      ) : (
        <div className="flex flex-wrap gap-2">
          {tags.data.map((t) => (
            <span key={t.id} className="inline-flex items-center gap-1.5 rounded-full border bg-card py-1 pr-1 pl-2.5 text-sm">
              <button
                type="button"
                disabled={!canManage}
                aria-label="Changer la couleur"
                className="size-2.5 rounded-full"
                style={{ background: t.color || "var(--muted-foreground)" }}
                onClick={() => recolor.mutate({ id: t.id, name: t.name, color: COLORS[(COLORS.indexOf(t.color) + 1) % COLORS.length] })}
              />
              {t.name}
              {canManage && (
                <Button variant="ghost" size="icon-sm" className="size-6" aria-label="Supprimer" onClick={() => remove.mutate(t.id)}>
                  <Trash2Icon className="size-3.5" />
                </Button>
              )}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
