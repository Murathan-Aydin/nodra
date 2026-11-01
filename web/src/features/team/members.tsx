import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { MailPlusIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import { api, get, post } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { teamQueries } from "@/lib/queries"
import { usePermission } from "@/lib/permissions"
import type { Scope } from "@/lib/permissions"

export type Role = { id: string; team_id: string | null; key: string; name: string; description: string; permissions: string[]; system: boolean }
type Member = { user_id: string; email: string; name: string; team_id: string; inherited: boolean; joined_at: string; roles: { assignment_id: string; role_name: string; effect: string }[] }
type Invitation = { id: string; email: string; role_id: string; role_name?: string; expires_at: string; created_at: string }
type Assignment = { id: string; user_id: string | null; group_id: string | null; subject_name: string; role_id: string; role_name: string; scope: Scope; effect: "allow" | "deny" }
export type Group = { id: string; name: string; description: string; members: { user_id: string; email: string; name: string }[] | null }

export const rolesQuery = (team: string) => ({ queryKey: ["team", team, "roles"], queryFn: () => get<Role[]>(`/teams/${team}/roles`) })
export const membersQuery = (team: string) => ({ queryKey: ["team", team, "members"], queryFn: () => get<Member[]>(`/teams/${team}/members`) })
export const groupsQuery = (team: string) => ({ queryKey: ["team", team, "groups"], queryFn: () => get<Group[]>(`/teams/${team}/groups`) })

/** Page « Équipe › Membres » (team.member.index du fork) : membres, invitations et attributions de rôles. */
export function MembersPage({ team }: { team: string }) {
  const manage = usePermission(team, "team.members.manage")
  return (
    <>
      <PageHeader title="Membres" description="Les droits viennent des rôles attribués à chaque membre ou à ses groupes, sur l'équipe ou sur une portée plus précise." />
      <div className="space-y-6">
        <MembersList team={team} manage={manage} />
        {manage && <Invitations team={team} />}
        <Assignments team={team} />
      </div>
    </>
  )
}

function MembersList({ team, manage }: { team: string; manage: boolean }) {
  const qc = useQueryClient()
  const members = useQuery(membersQuery(team))
  const [removing, setRemoving] = useState<Member | null>(null)
  const remove = useMutation({
    mutationFn: (m: Member) => api(`/teams/${team}/members/${m.user_id}`, { method: "DELETE" }),
    onSuccess: () => {
      setRemoving(null)
      void qc.invalidateQueries({ queryKey: ["team", team, "members"] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Membres" bodyClassName="p-0">
      {members.data?.map((m) => (
        <div key={m.user_id} className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">{(m.name || m.email).slice(0, 1).toUpperCase()}</span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{m.name || m.email}</p>
            <p className="truncate text-xs text-muted-foreground">
              {m.email} · depuis le {dateTime(m.joined_at)}
            </p>
          </div>
          {m.inherited && <Pill label="Hérité" dot="bg-muted-foreground" />}
          {m.roles.map((r) => (
            <Pill key={r.assignment_id} label={r.role_name} dot={r.effect === "deny" ? "bg-destructive" : "bg-primary"} />
          ))}
          {manage && !m.inherited && (
            <Button size="sm" variant="ghost" aria-label="Retirer" onClick={() => setRemoving(m)}>
              <Trash2Icon />
            </Button>
          )}
        </div>
      ))}
      <ConfirmDialog open={!!removing} onOpenChange={(o) => !o && setRemoving(null)} title={`Retirer ${removing?.email} ?`} description="Retiré aussi des sous-équipes." actionLabel="Retirer" loading={remove.isPending} onConfirm={() => removing && remove.mutate(removing)} />
    </SettingsCard>
  )
}

function Invitations({ team }: { team: string }) {
  const qc = useQueryClient()
  const roles = useQuery(rolesQuery(team)).data ?? []
  const invites = useQuery({ queryKey: ["team", team, "invitations"], queryFn: () => get<Invitation[]>(`/teams/${team}/invitations`) })
  const [d, setD] = useState({ email: "", role_id: "" })
  const [link, setLink] = useState("")
  const invite = useMutation({
    mutationFn: () => post<{ token: string }>(`/teams/${team}/invitations`, d),
    onSuccess: (r) => {
      setLink(`${location.origin}/invitations/accept?token=${r.token}`)
      setD({ email: "", role_id: d.role_id })
      toast.success("Invitation créée", { description: "Envoyée par e-mail si l'envoi est configuré." })
      void qc.invalidateQueries({ queryKey: ["team", team, "invitations"] })
    },
    onError: (e) => toast.error(e.message),
  })
  const revoke = useMutation({
    mutationFn: (id: string) => api(`/teams/${team}/invitations/${id}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["team", team, "invitations"] }),
    onError: (e) => toast.error(e.message),
  })
  const roleName = (id: string) => roles.find((r) => r.id === id)?.name ?? ""
  return (
    <SettingsCard title="Invitations" description="Valables 7 jours, liées à l'adresse invitée." bodyClassName="p-0">
      <form
        className="grid gap-3 border-b border-border p-4 sm:grid-cols-[1fr_14rem_auto] sm:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          invite.mutate()
        }}
      >
        <Field label="E-mail">
          <Input type="email" value={d.email} onChange={(e) => setD({ ...d, email: e.target.value })} required />
        </Field>
        <Field label="Rôle">
          <Select value={d.role_id} onValueChange={(v) => setD({ ...d, role_id: v })}>
            <SelectTrigger>
              <SelectValue placeholder="Choisir" />
            </SelectTrigger>
            <SelectContent>
              {roles
                .filter((r) => r.key !== "owner")
                .map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    {r.name}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
        </Field>
        <Button type="submit" size="sm" disabled={!d.email || !d.role_id} loading={invite.isPending}>
          <MailPlusIcon /> Inviter
        </Button>
      </form>
      {link && (
        <div className="border-b border-border p-4">
          <Field label="Lien d'invitation (à transmettre si l'e-mail n'est pas configuré)">
            <CopyInput value={link} />
          </Field>
        </div>
      )}
      {invites.data?.map((i) => (
        <div key={i.id} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm">{i.email}</p>
            <p className="text-xs text-muted-foreground">
              {i.role_name ?? roleName(i.role_id)} · expire {relativeTime(i.expires_at)}
            </p>
          </div>
          <Button size="sm" variant="ghost" onClick={() => revoke.mutate(i.id)}>
            Révoquer
          </Button>
        </div>
      ))}
      {invites.data?.length === 0 && <p className="px-5 py-4 text-sm text-muted-foreground">Aucune invitation en attente.</p>}
    </SettingsCard>
  )
}

const SCOPE_LABEL: Record<Scope["type"], string> = { team: "Équipe", project: "Projet", environment: "Environnement", server: "Serveur", resource: "Ressource" }

function Assignments({ team }: { team: string }) {
  const qc = useQueryClient()
  const manage = usePermission(team, "team.roles.manage")
  const list = useQuery({ queryKey: ["team", team, "role-assignments"], queryFn: () => get<Assignment[]>(`/teams/${team}/role-assignments`) })
  const roles = useQuery(rolesQuery(team)).data ?? []
  const members = useQuery(membersQuery(team)).data ?? []
  const groups = useQuery(groupsQuery(team)).data ?? []
  const projects = useQuery(teamQueries.projects(team)).data ?? []
  const servers = useQuery(teamQueries.servers(team)).data ?? []
  const resources = useQuery(teamQueries.resources(team)).data ?? []
  const [adding, setAdding] = useState(false)
  const [d, setD] = useState({ subject: "", role_id: "", scope: `team:${team}`, effect: "allow" })
  const scopeName = (s: Scope) =>
    s.type === "team" ? "toute l'équipe" : s.type === "project" ? projects.find((p) => p.id === s.id)?.name : s.type === "server" ? servers.find((x) => x.id === s.id)?.name : s.type === "resource" ? resources.find((r) => r.id === s.id)?.name : s.id.slice(0, 8)
  const add = useMutation({
    mutationFn: () => {
      const [kind, id] = d.subject.split(":")
      const [type, sid] = d.scope.split(":")
      return post(`/teams/${team}/role-assignments`, { [kind === "group" ? "group_id" : "user_id"]: id, role_id: d.role_id, scope: { type, id: sid }, effect: d.effect })
    },
    onSuccess: () => {
      setAdding(false)
      void qc.invalidateQueries({ queryKey: ["team", team] })
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: (id: string) => api(`/teams/${team}/role-assignments/${id}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["team", team] }),
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title="Attributions de rôles"
      description="La portée la plus précise l'emporte (ressource > environnement > projet > équipe) ; à portée égale, « refuser » l'emporte."
      actions={
        manage && (
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            <PlusIcon /> Attribuer
          </Button>
        )
      }
      bodyClassName="p-0"
    >
      {adding && (
        <div className="grid gap-3 border-b border-border p-4 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_1fr_9rem_auto] lg:items-end">
          <Field label="Membre ou groupe">
            <Select value={d.subject} onValueChange={(v) => setD({ ...d, subject: v })}>
              <SelectTrigger>
                <SelectValue placeholder="Choisir" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectLabel>Membres</SelectLabel>
                  {members.map((m) => (
                    <SelectItem key={m.user_id} value={`user:${m.user_id}`}>
                      {m.name || m.email}
                    </SelectItem>
                  ))}
                </SelectGroup>
                {groups.length > 0 && (
                  <SelectGroup>
                    <SelectLabel>Groupes</SelectLabel>
                    {groups.map((g) => (
                      <SelectItem key={g.id} value={`group:${g.id}`}>
                        {g.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                )}
              </SelectContent>
            </Select>
          </Field>
          <Field label="Rôle">
            <Select value={d.role_id} onValueChange={(v) => setD({ ...d, role_id: v })}>
              <SelectTrigger>
                <SelectValue placeholder="Choisir" />
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
          <Field label="Portée">
            <Select value={d.scope} onValueChange={(v) => setD({ ...d, scope: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={`team:${team}`}>Toute l'équipe</SelectItem>
                <SelectGroup>
                  <SelectLabel>Projets</SelectLabel>
                  {projects.map((p) => (
                    <SelectItem key={p.id} value={`project:${p.id}`}>
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
                <SelectGroup>
                  <SelectLabel>Serveurs</SelectLabel>
                  {servers.map((s) => (
                    <SelectItem key={s.id} value={`server:${s.id}`}>
                      {s.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
                <SelectGroup>
                  <SelectLabel>Ressources</SelectLabel>
                  {resources.map((r) => (
                    <SelectItem key={r.id} value={`resource:${r.id}`}>
                      {r.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field label="Effet">
            <Select value={d.effect} onValueChange={(v) => setD({ ...d, effect: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="allow">Autoriser</SelectItem>
                <SelectItem value="deny">Refuser</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <div className="flex gap-2">
            <Button size="sm" disabled={!d.subject || !d.role_id} loading={add.isPending} onClick={() => add.mutate()}>
              Attribuer
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setAdding(false)}>
              Annuler
            </Button>
          </div>
        </div>
      )}
      {list.data?.map((a) => (
        <div key={a.id} className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm">
              <span className="font-medium">{a.subject_name}</span> {a.group_id && <span className="text-xs text-muted-foreground">(groupe)</span>}
            </p>
            <p className="truncate text-xs text-muted-foreground">
              {SCOPE_LABEL[a.scope.type]} · {scopeName(a.scope) ?? a.scope.id}
            </p>
          </div>
          <Pill label={`${a.effect === "deny" ? "Refuse " : ""}${a.role_name}`} dot={a.effect === "deny" ? "bg-destructive" : "bg-primary"} />
          {manage && (
            <Button size="sm" variant="ghost" aria-label="Retirer" onClick={() => remove.mutate(a.id)}>
              <Trash2Icon />
            </Button>
          )}
        </div>
      ))}
    </SettingsCard>
  )
}
