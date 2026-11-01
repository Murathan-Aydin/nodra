import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { api, post } from "@/lib/api"
import { catalogQuery, usePermission } from "@/lib/permissions"
import { rolesQuery, type Role } from "./members"

/** Page « Équipe › Rôles » : rôles système (lecture seule) et rôles personnalisés. */
export function RolesPage({ team }: { team: string }) {
  const roles = useQuery(rolesQuery(team)).data ?? []
  const manage = usePermission(team, "team.roles.manage")
  const [adding, setAdding] = useState(false)
  return (
    <>
      <PageHeader
        title="Rôles"
        description="Un rôle regroupe des permissions ; on ne peut donner que des permissions que l'on détient soi-même."
        actions={
          manage && (
            <Button size="sm" onClick={() => setAdding(true)}>
              <PlusIcon /> Nouveau rôle
            </Button>
          )
        }
      />
      <div className="space-y-4">
        {adding && <RoleEditor team={team} onDone={() => setAdding(false)} />}
        {roles.map((r) => (
          <RoleCard key={r.id + r.permissions.join()} team={team} role={r} editable={manage && !r.system && r.team_id === team} />
        ))}
      </div>
    </>
  )
}

function RoleCard({ team, role, editable }: { team: string; role: Role; editable: boolean }) {
  const [open, setOpen] = useState(false)
  if (open && editable) return <RoleEditor team={team} role={role} onDone={() => setOpen(false)} />
  return (
    <SettingsCard
      title={role.name}
      description={role.description}
      actions={
        <>
          <Pill label={role.system ? "Système" : role.team_id === team ? "Personnalisé" : "Hérité"} dot={role.system ? "bg-muted-foreground" : "bg-primary"} />
          <Button size="sm" variant="ghost" onClick={() => setOpen(!open)}>
            {editable ? "Modifier" : open ? "Masquer" : "Permissions"}
          </Button>
        </>
      }
      bodyClassName={open ? "p-5" : "px-5 py-3"}
    >
      {open ? (
        <div className="flex flex-wrap gap-1.5">
          {role.permissions.map((p) => (
            <span key={p} className="rounded-md border border-border bg-muted px-2 py-0.5 font-mono text-[11px]">
              {p}
            </span>
          ))}
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">{role.permissions.includes("*") ? "Toutes les permissions" : `${role.permissions.length} permission(s)`}</p>
      )}
    </SettingsCard>
  )
}

function RoleEditor({ team, role, onDone }: { team: string; role?: Role; onDone: () => void }) {
  const qc = useQueryClient()
  const catalog = useQuery(catalogQuery).data
  const [name, setName] = useState(role?.name ?? "")
  const [description, setDescription] = useState(role?.description ?? "")
  const [perms, setPerms] = useState(() => new Set(role?.permissions ?? []))
  const [deleting, setDeleting] = useState(false)
  const save = useMutation({
    mutationFn: () => {
      const body = { name, description, permissions: [...perms] }
      return role ? api(`/teams/${team}/roles/${role.id}`, { method: "PATCH", json: body }) : post(`/teams/${team}/roles`, body)
    },
    onSuccess: () => {
      toast.success("Rôle enregistré")
      void qc.invalidateQueries({ queryKey: ["team", team, "roles"] })
      onDone()
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api(`/teams/${team}/roles/${role!.id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["team", team, "roles"] })
      onDone()
    },
    onError: (e) => toast.error(e.message),
  })
  const groups: [string, { key: string; description: string }[]][] = []
  for (const p of catalog?.permissions ?? []) {
    const g = groups.find(([name]) => name === p.group)
    if (g) g[1].push(p)
    else groups.push([p.group, [p]])
  }
  const toggle = (k: string, on: boolean) =>
    setPerms((s) => {
      const n = new Set(s)
      if (on) n.add(k)
      else n.delete(k)
      return n
    })
  return (
    <SettingsCard
      title={role ? `Modifier « ${role.name} »` : "Nouveau rôle"}
      actions={
        <>
          {role && (
            <Button size="sm" variant="ghost" onClick={() => setDeleting(true)}>
              <Trash2Icon /> Supprimer
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={onDone}>
            Annuler
          </Button>
          <Button size="sm" disabled={!name.trim() || perms.size === 0} loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
        </>
      }
    >
      <div className="mb-5 grid gap-4 sm:grid-cols-2">
        <Field label="Nom *">
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Description">
          <Input value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
      </div>
      <div className="grid gap-5 md:grid-cols-2">
        {groups.map(([group, list]) => (
          <div key={group}>
            <p className="mb-2 text-xs font-semibold text-muted-foreground">{group}</p>
            <div className="space-y-1.5">
              {list.map((p) => (
                <label key={p.key} className="flex items-start gap-2 text-sm">
                  <Checkbox className="mt-0.5" checked={perms.has(p.key)} onCheckedChange={(c) => toggle(p.key, c === true)} />
                  <span>
                    {p.description} <span className="font-mono text-[10px] text-muted-foreground">{p.key}</span>
                  </span>
                </label>
              ))}
            </div>
          </div>
        ))}
      </div>
      {role && <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer le rôle « ${role.name} » ?`} description="Ses attributions sont supprimées." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />}
    </SettingsCard>
  )
}
