import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { LockIcon, PencilIcon, PlusIcon, Trash2Icon, VariableIcon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { del, patch, post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { SharedVariable } from "@/lib/types"
import { variablesQuery } from "./queries"

const PREFIX: Record<string, string> = { team: "team", project: "project", environment: "environment", server: "server" }

interface Draft {
  id?: string
  key: string
  value: string
  comment: string
  is_secret: boolean
  is_multiline: boolean
  is_literal: boolean
}

const empty: Draft = { key: "", value: "", comment: "", is_secret: false, is_multiline: false, is_literal: false }

/** Variables partagées d'une portée (équipe par défaut), utilisables dans les ressources via {{portée.CLÉ}}. */
export function VariablesPanel({ team, scope }: { team: string; scope?: { type: string; id: string } }) {
  const vars = useQuery(variablesQuery(team, scope))
  const permScope = scope ?? undefined
  const canRead = usePermission(team, "variable.read", permScope)
  const canManage = usePermission(team, "variable.manage", permScope)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [deleting, setDeleting] = useState<SharedVariable | null>(null)
  const key = variablesQuery(team, scope).queryKey
  const save = useApiMutation(
    (d: Draft) => {
      const body = { key: d.key, comment: d.comment, is_secret: d.is_secret, is_multiline: d.is_multiline, is_literal: d.is_literal, ...(d.value || !d.id ? { value: d.value } : {}) }
      return d.id ? patch(teamPath(team, `/variables/${d.id}`), body) : post(teamPath(team, "/variables"), { ...body, scope })
    },
    { success: "Variable enregistrée", invalidate: [key], onSuccess: () => setDraft(null) },
  )
  const remove = useApiMutation((v: SharedVariable) => del(teamPath(team, `/variables/${v.id}`)), {
    success: "Variable supprimée",
    invalidate: [key],
    onSuccess: () => setDeleting(null),
  })
  const prefix = PREFIX[scope?.type ?? "team"]
  if (!canRead) return <p className="text-sm text-muted-foreground">Lecture des variables partagées non autorisée.</p>
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          Utilisables dans les variables des ressources : <code className="rounded bg-muted px-1 font-mono text-xs">{`{{${prefix}.CLÉ}}`}</code>
        </p>
        {canManage && (
          <Button size="sm" onClick={() => setDraft(empty)}>
            <PlusIcon /> Ajouter
          </Button>
        )}
      </div>
      {!vars.data?.length ? (
        <EmptyState icon={VariableIcon} title="Aucune variable partagée" />
      ) : (
        <Card>
          <ul className="divide-y">
            {vars.data.map((v) => (
              <li key={v.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
                <code className="font-mono font-medium">{v.key}</code>
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
                  {v.is_secret ? (
                    <span className="inline-flex items-center gap-1">
                      <LockIcon className="size-3" /> secret
                    </span>
                  ) : (
                    v.value
                  )}
                </span>
                {v.is_literal && <Badge variant="outline">littérale</Badge>}
                {v.comment && <span className="hidden max-w-48 truncate text-xs text-muted-foreground md:inline">{v.comment}</span>}
                {canManage && (
                  <>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label="Modifier"
                      onClick={() => setDraft({ id: v.id, key: v.key, value: v.is_secret ? "" : v.value, comment: v.comment, is_secret: v.is_secret, is_multiline: v.is_multiline, is_literal: v.is_literal })}
                    >
                      <PencilIcon />
                    </Button>
                    <Button variant="ghost" size="icon-sm" aria-label="Supprimer" onClick={() => setDeleting(v)}>
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </li>
            ))}
          </ul>
        </Card>
      )}
      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{draft?.id ? "Modifier la variable" : "Nouvelle variable"}</DialogTitle>
            {draft?.id && draft.is_secret && <DialogDescription>Laisser la valeur vide conserve le secret actuel.</DialogDescription>}
          </DialogHeader>
          {draft && (
            <form
              id="var-form"
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                save.mutate(draft)
              }}
            >
              <Field label="Clé" hint="Lettres majuscules, chiffres et _ (ex. DATABASE_URL).">
                <Input className="font-mono" required value={draft.key} onChange={(e) => setDraft({ ...draft, key: e.target.value.toUpperCase() })} />
              </Field>
              <Field label="Valeur">
                {draft.is_multiline ? (
                  <Textarea className="font-mono text-xs" rows={5} value={draft.value} onChange={(e) => setDraft({ ...draft, value: e.target.value })} />
                ) : (
                  <Input className="font-mono" type={draft.is_secret ? "password" : "text"} value={draft.value} onChange={(e) => setDraft({ ...draft, value: e.target.value })} />
                )}
              </Field>
              <Field label="Commentaire">
                <Input value={draft.comment} onChange={(e) => setDraft({ ...draft, comment: e.target.value })} />
              </Field>
              <div className="flex flex-wrap gap-4">
                {(
                  [
                    ["is_secret", "Secret (jamais réaffiché)"],
                    ["is_multiline", "Multiligne"],
                    ["is_literal", "Littérale (pas d'interpolation)"],
                  ] as const
                ).map(([k, label]) => (
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
            <Button type="submit" form="var-form" loading={save.isPending}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Supprimer ${deleting?.key} ?`}
        description="Les ressources qui l'utilisent la perdront au prochain déploiement."
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </div>
  )
}
