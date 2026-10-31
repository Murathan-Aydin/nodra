import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { PlusIcon, ShieldBanIcon, ShieldIcon, Trash2Icon, UserIcon, UsersRoundIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { del, post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { RoleAssignment, Scope } from "@/lib/types"
import { assignmentsQuery, groupsQuery, membersQuery, rolesQuery } from "./queries"
import { ScopePicker, useScopeNames } from "./scope-picker"

type Draft = { subject: "user" | "group"; subject_id: string; role_id: string; scope: Scope; effect: "allow" | "deny" }

/**
 * Attributions de rôles (utilisateur ou groupe × rôle × portée × autoriser/refuser).
 * Avec `scope`, n'affiche que les attributions de cette portée et la pré-remplit (onglet « Accès » d'une ressource).
 */
export function AccessPanel({ team, scope }: { team: string; scope?: Scope }) {
  const assignments = useQuery(assignmentsQuery(team))
  const members = useQuery(membersQuery(team))
  const groups = useQuery(groupsQuery(team))
  const roles = useQuery(rolesQuery(team))
  const names = useScopeNames(team)
  const canManage = usePermission(team, "team.roles.manage", scope)
  const [draft, setDraft] = useState<Draft | null>(null)
  const key = ["team", team, "role-assignments"]
  const create = useApiMutation(
    (d: Draft) =>
      post(teamPath(team, "/role-assignments"), {
        [d.subject === "user" ? "user_id" : "group_id"]: d.subject_id,
        role_id: d.role_id,
        scope: d.scope,
        effect: d.effect,
      }),
    { success: "Droit attribué", invalidate: [key, ["team", team, "members"]], onSuccess: () => setDraft(null) },
  )
  const remove = useApiMutation((a: RoleAssignment) => del(teamPath(team, `/role-assignments/${a.id}`)), {
    success: "Attribution retirée",
    invalidate: [key, ["team", team, "members"]],
  })
  const list = (assignments.data ?? []).filter((a) => !scope || (a.scope.type === scope.type && a.scope.id === scope.id))
  const scopeOK = (s: Scope) => s.type === "team" || !!s.id

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          La portée la plus précise l'emporte (ressource &gt; environnement &gt; projet &gt; équipe) ; à portée égale, un refus l'emporte.
        </p>
        {canManage && (
          <Button size="sm" onClick={() => setDraft({ subject: "user", subject_id: "", role_id: "", scope: scope ?? { type: "team", id: team }, effect: "allow" })}>
            <PlusIcon /> Attribuer
          </Button>
        )}
      </div>
      {!list.length ? (
        <EmptyState icon={ShieldIcon} title="Aucune attribution" description={scope ? "Les droits viennent des portées plus larges (environnement, projet, équipe)." : undefined} />
      ) : (
        <div className="rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Qui</TableHead>
                <TableHead>Rôle</TableHead>
                {!scope && <TableHead>Portée</TableHead>}
                <TableHead>Effet</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {list.map((a) => (
                <TableRow key={a.id}>
                  <TableCell>
                    <span className="inline-flex items-center gap-2">
                      {a.group_id ? <UsersRoundIcon className="size-4 text-muted-foreground" /> : <UserIcon className="size-4 text-muted-foreground" />}
                      {a.subject_name}
                    </span>
                  </TableCell>
                  <TableCell>{a.role_name}</TableCell>
                  {!scope && <TableCell className="text-muted-foreground">{names.label(a.scope)}</TableCell>}
                  <TableCell>
                    {a.effect === "deny" ? (
                      <Badge variant="destructive">
                        <ShieldBanIcon className="size-3" /> Refus
                      </Badge>
                    ) : (
                      <Badge variant="success">Autorisé</Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    {canManage && (
                      <Button variant="ghost" size="icon-sm" aria-label={`Retirer l'attribution de ${a.subject_name}`} onClick={() => remove.mutate(a)}>
                        <Trash2Icon />
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>Attribuer un rôle</DialogTitle>
            <DialogDescription>Vous ne pouvez attribuer qu'un rôle dont vous détenez toutes les permissions sur cette portée.</DialogDescription>
          </DialogHeader>
          {draft && (
            <div className="space-y-4">
              <div className="grid gap-2 sm:grid-cols-[10rem_1fr]">
                <Select value={draft.subject} onValueChange={(v) => setDraft({ ...draft, subject: v as Draft["subject"], subject_id: "" })}>
                  <SelectTrigger aria-label="Type de bénéficiaire">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">Membre</SelectItem>
                    <SelectItem value="group">Groupe</SelectItem>
                  </SelectContent>
                </Select>
                <Select value={draft.subject_id} onValueChange={(v) => setDraft({ ...draft, subject_id: v })}>
                  <SelectTrigger aria-label="Bénéficiaire">
                    <SelectValue placeholder="Choisir…" />
                  </SelectTrigger>
                  <SelectContent>
                    {draft.subject === "user"
                      ? members.data?.map((m) => (
                          <SelectItem key={m.user_id} value={m.user_id}>
                            {m.name || m.email} ({m.email})
                          </SelectItem>
                        ))
                      : groups.data?.map((g) => (
                          <SelectItem key={g.id} value={g.id}>
                            {g.name}
                          </SelectItem>
                        ))}
                  </SelectContent>
                </Select>
              </div>
              <Field label="Rôle">
                <Select value={draft.role_id} onValueChange={(v) => setDraft({ ...draft, role_id: v })}>
                  <SelectTrigger>
                    <SelectValue placeholder="Choisir…" />
                  </SelectTrigger>
                  <SelectContent>
                    {roles.data
                      ?.filter((r) => r.key !== "owner" || (draft.subject === "user" && draft.scope.type === "team"))
                      .map((r) => (
                        <SelectItem key={r.id} value={r.id}>
                          {r.name}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </Field>
              {!scope && (
                <div className="space-y-1.5">
                  <p className="text-sm font-medium">Portée</p>
                  <ScopePicker team={team} value={draft.scope} onChange={(s) => setDraft({ ...draft, scope: s })} />
                </div>
              )}
              <Field label="Effet">
                <Select value={draft.effect} onValueChange={(v) => setDraft({ ...draft, effect: v as Draft["effect"] })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="allow">Autoriser les permissions du rôle</SelectItem>
                    <SelectItem value="deny">Refuser les permissions du rôle (exception)</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button loading={create.isPending} disabled={!draft?.subject_id || !draft.role_id || !scopeOK(draft.scope)} onClick={() => draft && create.mutate(draft)}>
              Attribuer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
