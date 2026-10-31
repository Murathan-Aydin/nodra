import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { MailIcon, Trash2Icon, UserPlusIcon, UsersIcon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { CopyButton } from "@/components/copy-button"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { del, get, post, teamPath } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { meQuery, teamQuery } from "@/lib/queries"
import type { Invitation, Member, Role } from "@/lib/types"
import { groupsQuery, invitationsQuery, membersQuery, rolesQuery } from "@/features/team/queries"

export const Route = createFileRoute("/t/$team/team/")({ component: Members })

function Members() {
  const { team } = Route.useParams()
  const access = useQuery(teamQuery(team)).data
  const me = useQuery(meQuery).data
  const members = useQuery(membersQuery(team))
  const roles = useQuery(rolesQuery(team))
  const canManage = usePermission(team, "team.members.manage")
  const canAssign = usePermission(team, "team.roles.manage")
  const [removing, setRemoving] = useState<Member | null>(null)
  const [adding, setAdding] = useState(false)
  const invalidate = [["team", team, "members"], ["team", team, "role-assignments"]]

  // Rôle d'équipe : nouvelle attribution sur l'équipe puis retrait des anciennes attributions directes.
  const changeRole = useApiMutation(
    async ({ m, role }: { m: Member; role: string }) => {
      await post(teamPath(team, "/role-assignments"), { user_id: m.user_id, role_id: role, scope: { type: "team", id: team } })
      for (const r of m.roles.filter((r) => r.team_id === team && r.effect === "allow")) await del(teamPath(team, `/role-assignments/${r.assignment_id}`))
    },
    { success: "Rôle modifié", invalidate },
  )
  const remove = useApiMutation((m: Member) => del(teamPath(team, `/members/${m.user_id}`)), {
    success: "Membre retiré",
    invalidate,
    onSuccess: () => setRemoving(null),
  })

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex-row items-start justify-between gap-4">
          <div className="space-y-1.5">
            <CardTitle>Membres</CardTitle>
            <CardDescription>Les membres d'une équipe ont accès à ses sous-équipes ; les droits fins se règlent dans « Attributions ».</CardDescription>
          </div>
          {canManage && access?.team.parent_id && (
            <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
              <UserPlusIcon /> Ajouter depuis l'équipe parente
            </Button>
          )}
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Membre</TableHead>
                <TableHead>Rôle dans l'équipe</TableHead>
                <TableHead>Depuis</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {members.data?.map((m) => {
                const direct = m.roles.filter((r) => r.team_id === team && r.effect === "allow")
                const inheritedRoles = m.roles.filter((r) => r.team_id !== team)
                return (
                  <TableRow key={m.user_id}>
                    <TableCell>
                      <span className="block font-medium">
                        {m.name || m.email}
                        {m.user_id === me?.user.id && <span className="ml-2 text-xs text-muted-foreground">(vous)</span>}
                      </span>
                      <span className="block text-xs text-muted-foreground">{m.email}</span>
                    </TableCell>
                    <TableCell>
                      {canAssign && !m.inherited && m.user_id !== me?.user.id ? (
                        <Select value={direct[0]?.role_id ?? ""} onValueChange={(role) => changeRole.mutate({ m, role })}>
                          <SelectTrigger className="h-8 w-48" aria-label={`Rôle de ${m.email}`}>
                            <SelectValue placeholder="Aucun rôle" />
                          </SelectTrigger>
                          <SelectContent>
                            {roles.data?.map((r) => (
                              <SelectItem key={r.id} value={r.id}>
                                {r.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <span className="flex flex-wrap gap-1">
                          {direct.map((r) => (
                            <Badge key={r.assignment_id} variant="outline">
                              {r.role_name}
                            </Badge>
                          ))}
                          {!direct.length && !inheritedRoles.length && <span className="text-muted-foreground">—</span>}
                        </span>
                      )}
                      {inheritedRoles.length > 0 && (
                        <span className="mt-1 flex flex-wrap gap-1">
                          {inheritedRoles.map((r) => (
                            <Badge key={r.assignment_id} variant="outline" className="text-muted-foreground">
                              {r.role_name} (hérité)
                            </Badge>
                          ))}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {m.inherited ? <Badge variant="outline">Équipe parente</Badge> : <span title={dateTime(m.joined_at)}>{relativeTime(m.joined_at)}</span>}
                    </TableCell>
                    <TableCell className="text-right">
                      {canManage && !m.inherited && m.user_id !== me?.user.id && (
                        <Button variant="ghost" size="icon-sm" aria-label={`Retirer ${m.email}`} onClick={() => setRemoving(m)}>
                          <Trash2Icon />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {canManage && !access?.team.personal && <Invitations team={team} roles={roles.data ?? []} />}
      {access?.team.personal && (
        <p className="text-sm text-muted-foreground">Équipe personnelle : créez une organisation (sélecteur d'équipe) pour travailler à plusieurs.</p>
      )}

      {access?.team.parent_id && (
        <AddFromParent team={team} parent={access.team.parent_id} open={adding} onOpenChange={setAdding} roles={roles.data ?? []} current={members.data ?? []} />
      )}
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={`Retirer ${removing?.email} ?`}
        description="Ce membre perd l'accès à l'équipe et à ses sous-équipes."
        confirmLabel="Retirer"
        destructive
        loading={remove.isPending}
        onConfirm={() => removing && remove.mutate(removing)}
      />
    </div>
  )
}

function Invitations({ team, roles }: { team: string; roles: Role[] }) {
  const invitations = useQuery(invitationsQuery(team))
  const groups = useQuery(groupsQuery(team))
  const [draft, setDraft] = useState<{ email: string; role_id: string; group_ids: string[] } | null>(null)
  const [link, setLink] = useState<string | null>(null)
  const key = ["team", team, "invitations"]
  const invite = useApiMutation((d: NonNullable<typeof draft>) => post<{ invitation: Invitation; token: string }>(teamPath(team, "/invitations"), d), {
    success: "Invitation envoyée",
    invalidate: [key],
    onSuccess: (r) => {
      setDraft(null)
      setLink(`${window.location.origin}/invitations/accept?token=${encodeURIComponent(r.token)}`)
    },
  })
  const revoke = useApiMutation((id: string) => del(teamPath(team, `/invitations/${id}`)), { success: "Invitation annulée", invalidate: [key] })
  const defaultRole = roles.find((r) => r.key === "developer")?.id ?? roles[0]?.id ?? ""
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-4">
        <div className="space-y-1.5">
          <CardTitle>Invitations</CardTitle>
          <CardDescription>Valables 7 jours, liées à l'adresse e-mail invitée.</CardDescription>
        </div>
        <Button size="sm" onClick={() => setDraft({ email: "", role_id: defaultRole, group_ids: [] })}>
          <MailIcon /> Inviter
        </Button>
      </CardHeader>
      <CardContent>
        {!invitations.data?.length ? (
          <EmptyState icon={UsersIcon} title="Aucune invitation en attente" />
        ) : (
          <ul className="divide-y rounded-lg border">
            {invitations.data.map((i) => (
              <li key={i.id} className="flex items-center gap-3 px-3 py-2.5 text-sm">
                <MailIcon className="size-4 text-muted-foreground" />
                <span className="min-w-0 flex-1">
                  <span className="block font-medium">{i.email}</span>
                  <span className="block text-xs text-muted-foreground">
                    {i.role_name} · expire {relativeTime(i.expires_at)}
                  </span>
                </span>
                <Button variant="ghost" size="icon-sm" aria-label={`Annuler l'invitation de ${i.email}`} onClick={() => revoke.mutate(i.id)}>
                  <Trash2Icon />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Inviter un membre</DialogTitle>
            <DialogDescription>Un e-mail est envoyé si l'instance a un serveur SMTP ; le lien est aussi affiché ensuite.</DialogDescription>
          </DialogHeader>
          {draft && (
            <div className="space-y-4">
              <Field label="Adresse e-mail">
                <Input type="email" value={draft.email} onChange={(e) => setDraft({ ...draft, email: e.target.value })} autoFocus />
              </Field>
              <Field label="Rôle">
                <Select value={draft.role_id} onValueChange={(v) => setDraft({ ...draft, role_id: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {roles.map((r) => (
                      <SelectItem key={r.id} value={r.id}>
                        {r.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              {!!groups.data?.length && (
                <fieldset className="space-y-2">
                  <legend className="mb-2 text-sm font-medium">Groupes</legend>
                  {groups.data.map((g) => (
                    <Label key={g.id} className="font-normal">
                      <Checkbox
                        checked={draft.group_ids.includes(g.id)}
                        onCheckedChange={(c) => setDraft({ ...draft, group_ids: c === true ? [...draft.group_ids, g.id] : draft.group_ids.filter((x) => x !== g.id) })}
                      />
                      {g.name}
                    </Label>
                  ))}
                </fieldset>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button loading={invite.isPending} disabled={!draft?.email.includes("@") || !draft.role_id} onClick={() => draft && invite.mutate(draft)}>
              Inviter
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={!!link} onOpenChange={(o) => !o && setLink(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Lien d'invitation</DialogTitle>
            <DialogDescription>À transmettre à la personne invitée ; il ne sera plus affiché.</DialogDescription>
          </DialogHeader>
          <div className="flex items-start gap-2 rounded-lg bg-muted p-3">
            <code className="flex-1 font-mono text-xs break-all">{link}</code>
            <CopyButton value={link ?? ""} />
          </div>
          <DialogFooter>
            <Button onClick={() => setLink(null)}>Fermer</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

function AddFromParent({
  team,
  parent,
  open,
  onOpenChange,
  roles,
  current,
}: {
  team: string
  parent: string
  open: boolean
  onOpenChange: (o: boolean) => void
  roles: Role[]
  current: Member[]
}) {
  const candidates = useQuery({ queryKey: ["team", parent, "members"], queryFn: () => get<Member[]>(teamPath(parent, "/members")), enabled: open })
  const [user, setUser] = useState("")
  const [role, setRole] = useState("")
  const add = useApiMutation(() => post(teamPath(team, "/members"), { user_id: user, role_id: role }), {
    success: "Membre ajouté",
    invalidate: [["team", team, "members"]],
    onSuccess: () => {
      onOpenChange(false)
      setUser("")
    },
  })
  const direct = new Set(current.filter((m) => !m.inherited).map((m) => m.user_id))
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Ajouter un membre de l'équipe parente</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <Field label="Membre">
            <Select value={user} onValueChange={setUser}>
              <SelectTrigger>
                <SelectValue placeholder="Choisir…" />
              </SelectTrigger>
              <SelectContent>
                {candidates.data
                  ?.filter((m) => !direct.has(m.user_id))
                  .map((m) => (
                    <SelectItem key={m.user_id} value={m.user_id}>
                      {m.name || m.email} ({m.email})
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </Field>
          <Field label="Rôle">
            <Select value={role} onValueChange={setRole}>
              <SelectTrigger>
                <SelectValue placeholder="Choisir…" />
              </SelectTrigger>
              <SelectContent>
                {roles.map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    {r.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Annuler
          </Button>
          <Button loading={add.isPending} disabled={!user || !role} onClick={() => add.mutate(undefined)}>
            Ajouter
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
