import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { PlusIcon, Trash2Icon, XIcon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { EmptyPanel, PageHeader, SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { api, post } from "@/lib/api"
import { usePermission } from "@/lib/permissions"
import { groupsQuery, membersQuery, type Group } from "./members"

/** Page « Équipe › Groupes » : groupes de membres, auxquels on attribue des rôles. */
export function GroupsPage({ team }: { team: string }) {
  const qc = useQueryClient()
  const groups = useQuery(groupsQuery(team)).data ?? []
  const manage = usePermission(team, "team.members.manage")
  const [name, setName] = useState("")
  const create = useMutation({
    mutationFn: () => post(`/teams/${team}/groups`, { name }),
    onSuccess: () => {
      setName("")
      void qc.invalidateQueries({ queryKey: ["team", team, "groups"] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <>
      <PageHeader title="Groupes" description="Attribuez un rôle à un groupe (page Membres) : tous ses membres en héritent." />
      {manage && (
        <form
          className="glass-panel mb-6 flex max-w-md gap-2 rounded-2xl p-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (name.trim()) create.mutate()
          }}
        >
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Nouveau groupe" />
          <Button size="sm" type="submit" disabled={!name.trim()} loading={create.isPending}>
            <PlusIcon /> Créer
          </Button>
        </form>
      )}
      <div className="space-y-4">
        {groups.map((g) => (
          <GroupCard key={g.id} team={team} group={g} manage={manage} />
        ))}
        {groups.length === 0 && <EmptyPanel title="Aucun groupe" />}
      </div>
    </>
  )
}

function GroupCard({ team, group, manage }: { team: string; group: Group; manage: boolean }) {
  const qc = useQueryClient()
  const members = useQuery(membersQuery(team)).data ?? []
  const [user, setUser] = useState("")
  const [deleting, setDeleting] = useState(false)
  const refresh = () => void qc.invalidateQueries({ queryKey: ["team", team, "groups"] })
  const add = useMutation({ mutationFn: () => api(`/teams/${team}/groups/${group.id}/members/${user}`, { method: "PUT" }), onSuccess: () => (setUser(""), refresh()), onError: (e) => toast.error(e.message) })
  const out = useMutation({ mutationFn: (u: string) => api(`/teams/${team}/groups/${group.id}/members/${u}`, { method: "DELETE" }), onSuccess: refresh, onError: (e) => toast.error(e.message) })
  const remove = useMutation({ mutationFn: () => api(`/teams/${team}/groups/${group.id}`, { method: "DELETE" }), onSuccess: refresh, onError: (e) => toast.error(e.message) })
  const inGroup = new Set((group.members ?? []).map((m) => m.user_id))
  return (
    <SettingsCard
      title={group.name}
      description={group.description}
      actions={
        manage && (
          <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
            <Trash2Icon />
          </Button>
        )
      }
    >
      <div className="flex flex-wrap gap-2">
        {(group.members ?? []).map((m) => (
          <span key={m.user_id} className="inline-flex h-7 items-center gap-1.5 rounded-full border border-border bg-muted px-3 text-xs">
            {m.name || m.email}
            {manage && (
              <button type="button" aria-label="Retirer" onClick={() => out.mutate(m.user_id)} className="text-muted-foreground hover:text-foreground">
                <XIcon className="size-3" />
              </button>
            )}
          </span>
        ))}
        {!group.members?.length && <p className="text-sm text-muted-foreground">Aucun membre.</p>}
      </div>
      {manage && (
        <div className="mt-4 flex max-w-md gap-2">
          <Select value={user} onValueChange={setUser}>
            <SelectTrigger>
              <SelectValue placeholder="Ajouter un membre" />
            </SelectTrigger>
            <SelectContent>
              {members
                .filter((m) => !inGroup.has(m.user_id))
                .map((m) => (
                  <SelectItem key={m.user_id} value={m.user_id}>
                    {m.name || m.email}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
          <Button size="sm" variant="outline" disabled={!user} loading={add.isPending} onClick={() => add.mutate()}>
            Ajouter
          </Button>
        </div>
      )}
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer le groupe « ${group.name} » ?`} description="Ses attributions de rôles sont supprimées." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </SettingsCard>
  )
}
