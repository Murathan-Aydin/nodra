import { useQuery } from "@tanstack/react-query"
import { CheckIcon } from "lucide-react"
import { put } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import type { Tag } from "@/lib/types"
import { cn } from "@/lib/utils"
import { tagsQuery } from "@/features/projects/queries"

/** Tags d'une ressource : cliquer un tag l'ajoute ou le retire. */
export function TagPicker({ team, base, current, invalidate, disabled }: { team: string; base: string; current: Tag[]; invalidate: unknown[][]; disabled?: boolean }) {
  const tags = useQuery(tagsQuery(team))
  const ids = new Set(current.map((t) => t.id))
  const save = useApiMutation((tag_ids: string[]) => put(`${base}/tags`, { tag_ids }), {
    invalidate: [...invalidate, ["team", team, "resources"]],
  })
  if (!tags.data?.length) return <p className="text-sm text-muted-foreground">Aucun tag dans l'équipe (Projets → Tags).</p>
  return (
    <div className="flex flex-wrap gap-2">
      {tags.data.map((t) => {
        const on = ids.has(t.id)
        return (
          <button
            key={t.id}
            type="button"
            disabled={disabled || save.isPending}
            onClick={() => save.mutate(on ? [...ids].filter((x) => x !== t.id) : [...ids, t.id])}
            className={cn("inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-sm transition-colors disabled:opacity-60", on ? "border-primary bg-accent" : "hover:bg-muted")}
          >
            <span className="size-2 rounded-full" style={{ background: t.color || "var(--muted-foreground)" }} />
            {t.name}
            {on && <CheckIcon className="size-3.5" />}
          </button>
        )
      })}
    </div>
  )
}
