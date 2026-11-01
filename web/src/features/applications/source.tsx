import { useQuery } from "@tanstack/react-query"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import type { ResourceRef } from "@/features/resources/resource"
import { useDraft, useResourcePatch } from "@/features/resources/use-patch"
import { get } from "@/lib/api"
import type { Application } from "@/lib/types"
import { SaveButton } from "./general"

/** Onglet « Source Git » (Git Source du fork) : dépôt, branche, commit et chemins surveillés. */
export function ApplicationSource({ resource, app }: { resource: ResourceRef; app: Application }) {
  const sources = useQuery({ queryKey: ["team", resource.team, "sources"], queryFn: () => get<{ id: string; name: string; type: string }[]>(`/teams/${resource.team}/sources`) }).data ?? []
  const source = sources.find((s) => s.id === app.source_id)
  const { draft: d, set, changed, dirty } = useDraft({ git_repository: app.git_repository, git_branch: app.git_branch, git_commit_sha: app.git_commit_sha, watch_paths: app.watch_paths })
  const save = useResourcePatch(resource, "Source enregistrée (appliquée au prochain déploiement)")
  if (app.build_pack === "dockerimage") {
    return (
      <SettingsCard title="Source Git">
        <p className="text-sm text-muted-foreground">Cette application déploie une image Docker : pas de dépôt Git.</p>
      </SettingsCard>
    )
  }
  return (
    <SettingsCard
      title="Source Git"
      description={source ? `Via la source « ${source.name} » (${source.type}).` : app.private_key_id ? "Via une clé de déploiement SSH." : "Dépôt public."}
      actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(changed)} />}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Dépôt" hint={source ? "owner/nom" : "URL https ou git@hôte:owner/nom.git"} className="sm:col-span-2">
          <Input value={d.git_repository} onChange={(e) => set("git_repository", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Branche">
          <Input value={d.git_branch} onChange={(e) => set("git_branch", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Commit" hint="« HEAD » ou un SHA précis.">
          <Input value={d.git_commit_sha} onChange={(e) => set("git_commit_sha", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Chemins surveillés" hint="Un motif par ligne (*, **, ?, ! pour exclure) ; un push ne déploie que si un fichier modifié correspond." className="sm:col-span-2">
          <Textarea value={d.watch_paths} onChange={(e) => set("watch_paths", e.target.value)} rows={4} className="font-mono" />
        </Field>
      </div>
    </SettingsCard>
  )
}
