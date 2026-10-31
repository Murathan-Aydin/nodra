import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { FileTextIcon, LockIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { ApiError, del, get, patch, post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { EnvVar } from "@/lib/types"

type Flags = Pick<EnvVar, "is_buildtime" | "is_runtime" | "is_preview" | "is_literal" | "is_multiline" | "is_secret">

interface Draft extends Flags {
  id?: string
  key: string
  value: string
  comment: string
}

const FLAG_LABELS: [keyof Flags, string][] = [
  ["is_buildtime", "Disponible au build"],
  ["is_runtime", "Disponible à l'exécution"],
  ["is_secret", "Secret (jamais réaffiché)"],
  ["is_literal", "Littérale (sans interpolation)"],
  ["is_multiline", "Multiligne"],
]

/**
 * Variables d'environnement d'une ressource (`base` : chemin de la ressource dans l'API).
 * `previews` : onglet des variables de prévisualisation (applications uniquement).
 */
export function EnvPanel({ team, base, resource, previews }: { team: string; base: string; resource: string; previews?: boolean }) {
  const key = ["team", team, "envs", resource]
  const envs = useQuery({ queryKey: key, queryFn: () => get<EnvVar[]>(`${base}/envs`), retry: false })
  const scope = { type: "resource", id: resource }
  const canWrite = usePermission(team, "resource.env.write", scope)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [bulk, setBulk] = useState<{ content: string; is_preview: boolean } | null>(null)
  const save = useApiMutation(
    (d: Draft) => {
      const body: Record<string, unknown> = { key: d.key, comment: d.comment, is_buildtime: d.is_buildtime, is_runtime: d.is_runtime, is_preview: d.is_preview, is_literal: d.is_literal, is_multiline: d.is_multiline, is_secret: d.is_secret }
      if (!d.id || d.value || !d.is_secret) body.value = d.value
      return d.id ? patch(`${base}/envs/${d.id}`, body) : post(`${base}/envs`, body)
    },
    { success: (r) => (r ? "Variable enregistrée" : undefined), invalidate: [key], onSuccess: () => setDraft(null) },
  )
  const importBulk = useApiMutation((b: { content: string; is_preview: boolean }) => post(`${base}/envs/bulk`, b), {
    success: "Variables importées",
    invalidate: [key],
    onSuccess: () => setBulk(null),
  })
  const remove = useApiMutation((id: string) => del(`${base}/envs/${id}`), { success: "Variable supprimée", invalidate: [key] })

  if (envs.error instanceof ApiError && envs.error.status === 403)
    return <EmptyState icon={LockIcon} title="Variables non visibles" description="La permission « Lire les variables d'environnement et secrets » est nécessaire." />
  const list = (preview: boolean) => {
    const items = envs.data?.filter((e) => e.is_preview === preview) ?? []
    if (envs.isPending) return <Skeleton className="h-24 w-full" />
    if (!items.length) return <p className="py-6 text-center text-sm text-muted-foreground">Aucune variable{preview ? " de prévisualisation" : ""}.</p>
    return (
      <Card>
        <ul className="divide-y">
          {items.map((v) => (
            <li key={v.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
              <code className="shrink-0 font-mono font-medium">{v.key}</code>
              <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
                {v.is_secret ? (
                  <span className="inline-flex items-center gap-1">
                    <LockIcon className="size-3" /> secret
                  </span>
                ) : (
                  v.value
                )}
              </span>
              <span className="hidden gap-1 md:flex">
                {!v.is_buildtime && <Badge variant="outline">exécution</Badge>}
                {!v.is_runtime && <Badge variant="outline">build</Badge>}
                {v.is_literal && <Badge variant="outline">littérale</Badge>}
              </span>
              {canWrite && (
                <>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Modifier"
                    onClick={() => setDraft({ ...v, value: v.is_secret ? "" : v.value })}
                  >
                    <PencilIcon />
                  </Button>
                  <Button variant="ghost" size="icon-sm" aria-label="Supprimer" onClick={() => remove.mutate(v.id)}>
                    <Trash2Icon />
                  </Button>
                </>
              )}
            </li>
          ))}
        </ul>
      </Card>
    )
  }
  const newDraft = (preview: boolean): Draft => ({ key: "", value: "", comment: "", is_buildtime: true, is_runtime: true, is_preview: preview, is_literal: false, is_multiline: false, is_secret: false })
  const toolbar = (preview: boolean) =>
    canWrite && (
      <div className="flex justify-end gap-2">
        <Button size="sm" variant="outline" onClick={() => setBulk({ content: "", is_preview: preview })}>
          <FileTextIcon /> Importer un .env
        </Button>
        <Button size="sm" onClick={() => setDraft(newDraft(preview))}>
          <PlusIcon /> Ajouter
        </Button>
      </div>
    )
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        Les valeurs peuvent référencer les variables partagées : <code className="font-mono text-xs">{"{{team.X}}"}</code>, <code className="font-mono text-xs">{"{{project.X}}"}</code>,{" "}
        <code className="font-mono text-xs">{"{{environment.X}}"}</code>, <code className="font-mono text-xs">{"{{server.X}}"}</code>. Appliquées au prochain déploiement.
      </p>
      {previews ? (
        <Tabs defaultValue="prod">
          <TabsList>
            <TabsTrigger value="prod">Production</TabsTrigger>
            <TabsTrigger value="preview">Prévisualisations</TabsTrigger>
          </TabsList>
          <TabsContent value="prod" className="space-y-3">
            {toolbar(false)}
            {list(false)}
          </TabsContent>
          <TabsContent value="preview" className="space-y-3">
            {toolbar(true)}
            {list(true)}
          </TabsContent>
        </Tabs>
      ) : (
        <>
          {toolbar(false)}
          {list(false)}
        </>
      )}

      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{draft?.id ? "Modifier la variable" : "Nouvelle variable"}</DialogTitle>
            {draft?.id && draft.is_secret && <DialogDescription>Laisser la valeur vide conserve le secret actuel.</DialogDescription>}
          </DialogHeader>
          {draft && (
            <form
              id="env-form"
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                save.mutate(draft)
              }}
            >
              <Field label="Clé">
                <Input className="font-mono" required value={draft.key} onChange={(e) => setDraft({ ...draft, key: e.target.value })} />
              </Field>
              <Field label="Valeur">
                {draft.is_multiline ? (
                  <Textarea rows={5} className="font-mono text-xs" value={draft.value} onChange={(e) => setDraft({ ...draft, value: e.target.value })} />
                ) : (
                  <Input className="font-mono" type={draft.is_secret ? "password" : "text"} value={draft.value} onChange={(e) => setDraft({ ...draft, value: e.target.value })} />
                )}
              </Field>
              <Field label="Commentaire">
                <Input value={draft.comment} onChange={(e) => setDraft({ ...draft, comment: e.target.value })} />
              </Field>
              <div className="grid gap-2 sm:grid-cols-2">
                {FLAG_LABELS.map(([k, label]) => (
                  <Label key={k} className="font-normal">
                    <Checkbox checked={draft[k]} onCheckedChange={(c) => setDraft({ ...draft, [k]: c === true })} /> {label}
                  </Label>
                ))}
              </div>
            </form>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button type="submit" form="env-form" loading={save.isPending}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!bulk} onOpenChange={(o) => !o && setBulk(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Importer un fichier .env</DialogTitle>
            <DialogDescription>Une variable par ligne (CLÉ=valeur) ; les clés existantes sont remplacées.</DialogDescription>
          </DialogHeader>
          {bulk && <Textarea rows={12} className="font-mono text-xs" placeholder={"DATABASE_URL=postgres://…\nNODE_ENV=production"} value={bulk.content} onChange={(e) => setBulk({ ...bulk, content: e.target.value })} />}
          <DialogFooter>
            <Button variant="outline" onClick={() => setBulk(null)}>
              Annuler
            </Button>
            <Button loading={importBulk.isPending} onClick={() => bulk && importBulk.mutate(bulk)}>
              Importer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
