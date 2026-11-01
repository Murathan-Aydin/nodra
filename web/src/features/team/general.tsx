import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { PlusIcon, UsersIcon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { SaveButton } from "@/features/applications/general"
import { useDraft } from "@/features/resources/use-patch"
import { api, ApiError, get, post } from "@/lib/api"
import { teamsQuery } from "@/lib/queries"
import type { Team } from "@/lib/types"

export type TeamDetail = { team: Team & { description: string; personal: boolean; parent_id: string | null }; ancestors: Team[]; permissions: string[]; is_owner: boolean }
export const teamDetailQuery = (team: string) => ({ queryKey: ["team", team, "detail"], queryFn: () => get<TeamDetail>(`/teams/${team}`) })
type Member = { user_id: string; email: string; name: string; inherited: boolean; roles: { role_name: string }[] }

/** Page « Équipe › Général » (team.index du fork) : nom, description, sous-équipes, propriété. */
export function TeamGeneralPage({ team }: { team: string }) {
  const detail = useQuery(teamDetailQuery(team)).data
  const teams = useQuery(teamsQuery).data ?? []
  if (!detail) return null
  const children = teams.filter((t) => t.parent_id === team)
  return (
    <>
      <PageHeader title="Équipe" description={detail.ancestors.length ? `Sous-équipe de ${detail.ancestors.map((a) => a.name).join(" › ")}` : detail.team.personal ? "Équipe personnelle" : "Organisation"} />
      <div className="space-y-6">
        <TeamInfo key={detail.team.name + detail.team.description} team={team} detail={detail} />
        <SubTeams team={team} subteams={children} />
        {detail.is_owner && !detail.team.personal && <Transfer team={team} />}
      </div>
    </>
  )
}

function TeamInfo({ team, detail }: { team: string; detail: TeamDetail }) {
  const qc = useQueryClient()
  const { draft: d, set, changed, dirty } = useDraft({ name: detail.team.name, description: detail.team.description ?? "" })
  const save = useMutation({
    mutationFn: () => api(`/teams/${team}`, { method: "PATCH", json: { name: d.name, description: d.description, ...changed } }),
    onSuccess: () => {
      toast.success("Équipe enregistrée")
      void qc.invalidateQueries({ queryKey: ["teams"] })
      void qc.invalidateQueries({ queryKey: ["team", team, "detail"] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Détails de l'équipe" actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate()} />}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Nom *">
          <Input value={d.name} onChange={(e) => set("name", e.target.value)} />
        </Field>
        <Field label="Description">
          <Input value={d.description} onChange={(e) => set("description", e.target.value)} />
        </Field>
      </div>
    </SettingsCard>
  )
}

function SubTeams({ team, subteams }: { team: string; subteams: Team[] }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [name, setName] = useState("")
  const create = useMutation({
    mutationFn: () => post<Team>("/teams", { name, parent_id: team }),
    onSuccess: (t) => {
      setName("")
      void qc.invalidateQueries({ queryKey: ["teams"] })
      void navigate({ to: "/t/$team", params: { team: t.id } })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Sous-équipes" description="Les membres d'une équipe accèdent à toutes ses sous-équipes ; un membre d'une sous-équipe ne voit pas l'équipe parente (5 niveaux max.).">
      <div className="space-y-2">
        {subteams.map((t) => (
          <Link key={t.id} to="/t/$team" params={{ team: t.id }} className="flex items-center gap-3 rounded-lg border border-border px-3 py-2 text-sm hover:bg-foreground/[0.03]">
            <UsersIcon className="size-4 text-muted-foreground" /> {t.name}
          </Link>
        ))}
        <form
          className="flex max-w-md gap-2 pt-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (name.trim()) create.mutate()
          }}
        >
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Nom de la sous-équipe" />
          <Button size="sm" type="submit" variant="outline" disabled={!name.trim()} loading={create.isPending}>
            <PlusIcon /> Créer
          </Button>
        </form>
      </div>
    </SettingsCard>
  )
}

function Transfer({ team }: { team: string }) {
  const qc = useQueryClient()
  const members = useQuery({ queryKey: ["team", team, "members"], queryFn: () => get<Member[]>(`/teams/${team}/members`) }).data ?? []
  const [user, setUser] = useState("")
  const [confirm, setConfirm] = useState(false)
  const transfer = useMutation({
    mutationFn: () => post(`/teams/${team}/transfer-ownership`, { user_id: user }),
    onSuccess: () => {
      setConfirm(false)
      toast.success("Propriété transférée : vous êtes maintenant administrateur")
      void qc.invalidateQueries({ queryKey: ["team", team] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Transférer la propriété" description="Le nouveau propriétaire obtient tous les droits ; vous devenez administrateur.">
      <div className="flex max-w-lg gap-2">
        <Select value={user} onValueChange={setUser}>
          <SelectTrigger>
            <SelectValue placeholder="Choisir un membre" />
          </SelectTrigger>
          <SelectContent>
            {members
              .filter((m) => !m.inherited)
              .map((m) => (
                <SelectItem key={m.user_id} value={m.user_id}>
                  {m.name || m.email}
                </SelectItem>
              ))}
          </SelectContent>
        </Select>
        <Button size="sm" variant="outline" disabled={!user} onClick={() => setConfirm(true)}>
          Transférer
        </Button>
      </div>
      <ConfirmDialog open={confirm} onOpenChange={setConfirm} title="Transférer la propriété ?" actionLabel="Transférer" loading={transfer.isPending} onConfirm={() => transfer.mutate()} />
    </SettingsCard>
  )
}

/** Page « Équipe › Zone de danger » : quitter ou supprimer l'équipe. */
export function TeamDangerPage({ team }: { team: string }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const detail = useQuery(teamDetailQuery(team)).data
  const [leaving, setLeaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [withResources, setWithResources] = useState(false)
  const [blocking, setBlocking] = useState<{ name: string }[] | null>(null)
  const leave = useMutation({
    mutationFn: () => post(`/teams/${team}/leave`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["teams"] })
      void navigate({ to: "/" })
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api(`/teams/${team}`, { method: "DELETE", json: withResources ? { delete_resources: true } : {} }),
    onSuccess: () => {
      toast.success("Équipe supprimée")
      void qc.invalidateQueries({ queryKey: ["teams"] })
      void navigate({ to: "/" })
    },
    onError: (e) => {
      const res = e instanceof ApiError ? (e.body as { resources?: { name: string }[] } | undefined)?.resources : undefined
      if (res) setBlocking(res)
      toast.error(e.message)
    },
  })
  if (!detail) return null
  const personal = detail.team.personal
  return (
    <>
      <PageHeader title="Zone de danger" />
      <SettingsCard title="Zone de danger">
        <div className="space-y-5">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div>
              <p className="text-sm font-semibold">Quitter l'équipe</p>
              <p className="text-sm text-muted-foreground">{personal ? "Impossible de quitter son équipe personnelle." : "Impossible pour le dernier propriétaire."}</p>
            </div>
            <Button size="sm" variant="outline" disabled={personal} onClick={() => setLeaving(true)}>
              Quitter
            </Button>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-4 border-t border-border pt-5">
            <div>
              <p className="text-sm font-semibold">Supprimer l'équipe</p>
              <p className="text-sm text-muted-foreground">{personal ? "Une équipe personnelle ne peut pas être supprimée." : "Refusé tant qu'il reste des ressources, sauf si vous choisissez de les supprimer aussi."}</p>
            </div>
            <Button size="sm" variant="destructive" disabled={personal} onClick={() => setDeleting(true)}>
              Supprimer
            </Button>
          </div>
          {blocking && (
            <p className="text-sm text-destructive">
              Ressources restantes : {blocking.map((r) => r.name).join(", ")}
            </p>
          )}
        </div>
      </SettingsCard>
      <ConfirmDialog open={leaving} onOpenChange={setLeaving} title="Quitter l'équipe ?" actionLabel="Quitter" loading={leave.isPending} onConfirm={() => leave.mutate()} />
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer « ${detail.team.name} » ?`} confirmText={detail.team.name} actionLabel="Supprimer définitivement" loading={remove.isPending} onConfirm={() => remove.mutate()}>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={withResources} onCheckedChange={(c) => setWithResources(c === true)} />
          Supprimer aussi toutes les ressources (arrêtées et effacées des serveurs, volumes compris)
        </label>
      </ConfirmDialog>
    </>
  )
}
