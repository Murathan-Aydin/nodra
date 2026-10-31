import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { PencilIcon, PlusIcon, Trash2Icon, UsersRoundIcon, XIcon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { del, patch, post, put, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Group } from "@/lib/types"
import { groupsQuery, membersQuery } from "@/features/team/queries"

export const Route = createFileRoute("/t/$team/team/groups")({ component: Groups })

function Groups() {
  const { team } = Route.useParams()
  const groups = useQuery(groupsQuery(team))
  const members = useQuery(membersQuery(team))
  const canManage = usePermission(team, "team.members.manage")
  const [draft, setDraft] = useState<{ id?: string; name: string; description: string } | null>(null)
  const [deleting, setDeleting] = useState<Group | null>(null)
  const key = ["team", team, "groups"]
  const save = useApiMutation(
    (d: NonNullable<typeof draft>) =>
      d.id ? patch(teamPath(team, `/groups/${d.id}`), { name: d.name, description: d.description }) : post(teamPath(team, "/groups"), { name: d.name, description: d.description }),
    { success: "Groupe enregistré", invalidate: [key], onSuccess: () => setDraft(null) },
  )
  const remove = useApiMutation((id: string) => del(teamPath(team, `/groups/${id}`)), { success: "Groupe supprimé", invalidate: [key], onSuccess: () => setDeleting(null) })
  const addMember = useApiMutation(({ g, user }: { g: string; user: string }) => put(teamPath(team, `/groups/${g}/members/${user}`), {}), { invalidate: [key] })
  const removeMember = useApiMutation(({ g, user }: { g: string; user: string }) => del(teamPath(team, `/groups/${g}/members/${user}`)), { invalidate: [key] })

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">Attribuez un rôle à un groupe (onglet « Attributions ») plutôt qu'à chaque membre.</p>
        {canManage && (
          <Button size="sm" onClick={() => setDraft({ name: "", description: "" })}>
            <PlusIcon /> Nouveau groupe
          </Button>
        )}
      </div>
      {!groups.data?.length ? (
        <EmptyState icon={UsersRoundIcon} title="Aucun groupe" />
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {groups.data.map((g) => {
            const inGroup = new Set(g.members.map((m) => m.user_id))
            const candidates = (members.data ?? []).filter((m) => !inGroup.has(m.user_id))
            return (
              <Card key={g.id}>
                <CardHeader className="flex-row items-start justify-between gap-2">
                  <div className="space-y-1.5">
                    <CardTitle>{g.name}</CardTitle>
                    {g.description && <CardDescription>{g.description}</CardDescription>}
                  </div>
                  {canManage && (
                    <div className="flex">
                      <Button variant="ghost" size="icon-sm" aria-label={`Modifier ${g.name}`} onClick={() => setDraft({ id: g.id, name: g.name, description: g.description })}>
                        <PencilIcon />
                      </Button>
                      <Button variant="ghost" size="icon-sm" aria-label={`Supprimer ${g.name}`} onClick={() => setDeleting(g)}>
                        <Trash2Icon />
                      </Button>
                    </div>
                  )}
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex flex-wrap gap-1.5">
                    {g.members.map((m) => (
                      <Badge key={m.user_id} variant="outline" className="gap-1">
                        {m.name || m.email}
                        {canManage && (
                          <button type="button" aria-label={`Retirer ${m.email}`} onClick={() => removeMember.mutate({ g: g.id, user: m.user_id })} className="hover:text-destructive">
                            <XIcon className="size-3" />
                          </button>
                        )}
                      </Badge>
                    ))}
                    {!g.members.length && <span className="text-sm text-muted-foreground">Aucun membre.</span>}
                  </div>
                  {canManage && candidates.length > 0 && (
                    <Select value="" onValueChange={(user) => addMember.mutate({ g: g.id, user })}>
                      <SelectTrigger className="h-8 w-60" aria-label={`Ajouter un membre à ${g.name}`}>
                        <SelectValue placeholder="Ajouter un membre…" />
                      </SelectTrigger>
                      <SelectContent>
                        {candidates.map((m) => (
                          <SelectItem key={m.user_id} value={m.user_id}>
                            {m.name || m.email}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{draft?.id ? "Modifier le groupe" : "Nouveau groupe"}</DialogTitle>
          </DialogHeader>
          {draft && (
            <div className="space-y-4">
              <Field label="Nom">
                <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} autoFocus />
              </Field>
              <Field label="Description">
                <Input value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
              </Field>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button loading={save.isPending} disabled={!draft?.name.trim()} onClick={() => draft && save.mutate(draft)}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Supprimer le groupe ${deleting?.name} ?`}
        description="Les droits attribués au groupe sont retirés à ses membres."
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </div>
  )
}
