import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useMemo, useState } from "react"
import { EyeIcon, LockIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { del, patch, post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { permissionCatalogQuery } from "@/lib/queries"
import type { Permission, Role } from "@/lib/types"
import { compressPermissions, expandPermissions } from "@/features/team/permissions"
import { rolesQuery } from "@/features/team/queries"

export const Route = createFileRoute("/t/$team/team/roles")({ component: Roles })

type Draft = { id?: string; name: string; description: string; perms: Set<string>; readOnly?: boolean }

function Roles() {
  const { team } = Route.useParams()
  const roles = useQuery(rolesQuery(team))
  const catalog = useQuery(permissionCatalogQuery)
  const canManage = usePermission(team, "team.roles.manage")
  const perms = catalog.data?.permissions ?? []
  const [draft, setDraft] = useState<Draft | null>(null)
  const [deleting, setDeleting] = useState<Role | null>(null)
  const key = ["team", team, "roles"]
  const save = useApiMutation(
    (d: Draft) => {
      const body = { name: d.name.trim(), description: d.description, permissions: compressPermissions(d.perms, perms) }
      return d.id ? patch(teamPath(team, `/roles/${d.id}`), body) : post(teamPath(team, "/roles"), body)
    },
    { success: "Rôle enregistré", invalidate: [key], onSuccess: () => setDraft(null) },
  )
  const remove = useApiMutation((id: string) => del(teamPath(team, `/roles/${id}`)), { success: "Rôle supprimé", invalidate: [key], onSuccess: () => setDeleting(null) })
  const open = (r: Role, readOnly: boolean) => setDraft({ id: r.id, name: r.name, description: r.description, perms: expandPermissions(r.permissions, perms), readOnly })

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">Rôles système non modifiables ; les rôles personnalisés sont visibles par les sous-équipes.</p>
        {canManage && (
          <Button size="sm" onClick={() => setDraft({ name: "", description: "", perms: new Set(["team.read", "project.read", "server.read", "resource.read"]) })}>
            <PlusIcon /> Nouveau rôle
          </Button>
        )}
      </div>
      <Card>
        <ul className="divide-y">
          {roles.data?.map((r) => {
            const own = r.team_id === team
            return (
              <li key={r.id} className="flex flex-wrap items-center gap-3 px-4 py-3 text-sm">
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-2 font-medium">
                    {r.name}
                    {r.system ? (
                      <Badge variant="outline">
                        <LockIcon className="size-3" /> système
                      </Badge>
                    ) : (
                      !own && <Badge variant="outline">équipe parente</Badge>
                    )}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {r.description || `${r.permissions.includes("*") ? "Toutes les" : r.permissions.length} permissions`}
                  </span>
                </span>
                {canManage && own && !r.system ? (
                  <>
                    <Button variant="ghost" size="icon-sm" aria-label={`Modifier ${r.name}`} onClick={() => open(r, false)}>
                      <PencilIcon />
                    </Button>
                    <Button variant="ghost" size="icon-sm" aria-label={`Supprimer ${r.name}`} onClick={() => setDeleting(r)}>
                      <Trash2Icon />
                    </Button>
                  </>
                ) : (
                  <Button variant="ghost" size="icon-sm" aria-label={`Voir ${r.name}`} onClick={() => open(r, true)}>
                    <EyeIcon />
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      </Card>
      <RoleDialog draft={draft} onChange={setDraft} perms={perms} saving={save.isPending} onSave={(d) => save.mutate(d)} />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Supprimer le rôle ${deleting?.name} ?`}
        description="Impossible tant qu'il est attribué ou utilisé par une invitation."
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </div>
  )
}

function RoleDialog({
  draft,
  onChange,
  perms,
  saving,
  onSave,
}: {
  draft: Draft | null
  onChange: (d: Draft | null) => void
  perms: Permission[]
  saving: boolean
  onSave: (d: Draft) => void
}) {
  const groups = useMemo(() => {
    const m = new Map<string, Permission[]>()
    for (const p of perms) m.set(p.group, [...(m.get(p.group) ?? []), p])
    return [...m.entries()]
  }, [perms])
  const toggle = (keys: string[], on: boolean) => {
    if (!draft) return
    const next = new Set(draft.perms)
    for (const k of keys) {
      if (on) next.add(k)
      else next.delete(k)
    }
    onChange({ ...draft, perms: next })
  }
  const ro = !!draft?.readOnly
  return (
    <Dialog open={!!draft} onOpenChange={(o) => !o && onChange(null)}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{ro ? draft?.name : draft?.id ? "Modifier le rôle" : "Nouveau rôle"}</DialogTitle>
          <DialogDescription>Vous ne pouvez accorder que des permissions que vous détenez vous-même.</DialogDescription>
        </DialogHeader>
        {draft && (
          <div className="space-y-5">
            {!ro && (
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Nom">
                  <Input value={draft.name} onChange={(e) => onChange({ ...draft, name: e.target.value })} placeholder="ex. Déployeur" autoFocus />
                </Field>
                <Field label="Description">
                  <Input value={draft.description} onChange={(e) => onChange({ ...draft, description: e.target.value })} />
                </Field>
              </div>
            )}
            <div className="grid gap-4 md:grid-cols-2">
              {groups.map(([group, list]) => {
                const keys = list.map((p) => p.key)
                const count = keys.filter((k) => draft.perms.has(k)).length
                return (
                  <fieldset key={group} className="rounded-lg border p-3">
                    <legend className="px-1">
                      <Label className="font-medium">
                        <Checkbox
                          checked={count === keys.length ? true : count ? "indeterminate" : false}
                          onCheckedChange={(c) => toggle(keys, c === true)}
                          disabled={ro}
                        />
                        {group}
                      </Label>
                    </legend>
                    <div className="space-y-1.5">
                      {list.map((p) => (
                        <Label key={p.key} className="items-start font-normal" title={p.key}>
                          <Checkbox className="mt-0.5" checked={draft.perms.has(p.key)} onCheckedChange={(c) => toggle([p.key], c === true)} disabled={ro} />
                          <span>
                            {p.description}
                            <code className="ml-1 font-mono text-[10px] text-muted-foreground">{p.key}</code>
                          </span>
                        </Label>
                      ))}
                    </div>
                  </fieldset>
                )
              })}
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onChange(null)}>
            {ro ? "Fermer" : "Annuler"}
          </Button>
          {!ro && (
            <Button loading={saving} disabled={!draft?.name.trim() || !draft.perms.size} onClick={() => draft && onSave(draft)}>
              Enregistrer
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
