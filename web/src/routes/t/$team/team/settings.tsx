import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ApiError, del, patch, post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { meQuery, teamQuery, teamsQuery } from "@/lib/queries"
import type { Team } from "@/lib/types"
import { membersQuery } from "@/features/team/queries"

export const Route = createFileRoute("/t/$team/team/settings")({ component: TeamSettings })

function TeamSettings() {
  const { team } = Route.useParams()
  const access = useQuery(teamQuery(team)).data
  if (!access) return null
  return <SettingsForm key={access.team.id + access.team.name} team={access.team} isOwner={access.is_owner} />
}

function SettingsForm({ team: t, isOwner }: { team: Team; isOwner: boolean }) {
  const navigate = useNavigate()
  const team = t.id
  const teams = useQuery(teamsQuery)
  const me = useQuery(meQuery).data
  const members = useQuery(membersQuery(team))
  const canUpdate = usePermission(team, "team.update")
  const canDelete = usePermission(team, "team.delete")
  const [f, setF] = useState({ name: t.name, description: t.description })
  const [newOwner, setNewOwner] = useState("")
  const [confirm, setConfirm] = useState<"leave" | "transfer" | "delete" | "delete-resources" | null>(null)
  const [resources, setResources] = useState(0)
  const personal = teams.data?.find((x) => x.personal)
  const goHome = () => personal && void navigate({ to: "/t/$team", params: { team: personal.id } })

  const save = useApiMutation(() => patch(teamPath(team), f), { success: "Équipe enregistrée", invalidate: [["teams"], ["team", team, "access"]] })
  const leave = useApiMutation(() => post(teamPath(team, "/leave")), { success: "Vous avez quitté l'équipe", invalidate: [["teams"]], onSuccess: goHome })
  const transfer = useApiMutation(() => post(teamPath(team, "/transfer-ownership"), { user_id: newOwner }), {
    success: "Propriété transférée ; vous êtes désormais administrateur",
    invalidate: [["team", team]],
    onSuccess: () => setConfirm(null),
  })
  const remove = useApiMutation(
    async (withResources: boolean) => {
      try {
        return await del<undefined>(teamPath(team), withResources ? { delete_resources: true } : undefined)
      } catch (e) {
        // L'équipe a encore des ressources : demander une confirmation explicite.
        const count = e instanceof ApiError && e.status === 409 ? (e.body as { resources?: number } | undefined)?.resources : undefined
        if (count) {
          setResources(count)
          setConfirm("delete-resources")
          return null
        }
        throw e
      }
    },
    {
      success: (r) => (r === null ? undefined : "Équipe supprimée"),
      invalidate: [["teams"]],
      onSuccess: (r) => {
        if (r === null) return
        setConfirm(null)
        goHome()
      },
    },
  )
  const candidates = (members.data ?? []).filter((m) => !m.inherited && m.user_id !== me?.user.id)

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Informations</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              save.mutate(undefined)
            }}
          >
            <fieldset disabled={!canUpdate} className="grid gap-4 md:grid-cols-2">
              <Field label="Nom">
                <Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
              </Field>
              <Field label="Description">
                <Input value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
              </Field>
            </fieldset>
            {canUpdate && (
              <div className="mt-4 flex justify-end">
                <Button type="submit" loading={save.isPending} disabled={!f.name.trim()}>
                  Enregistrer
                </Button>
              </div>
            )}
          </form>
        </CardContent>
      </Card>

      {isOwner && !t.personal && (
        <Card>
          <CardHeader>
            <CardTitle>Transférer la propriété</CardTitle>
            <CardDescription>Le nouveau propriétaire obtient tous les droits ; vous devenez administrateur.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            <Select value={newOwner} onValueChange={setNewOwner}>
              <SelectTrigger className="w-72" aria-label="Nouveau propriétaire">
                <SelectValue placeholder={candidates.length ? "Choisir un membre…" : "Aucun autre membre direct"} />
              </SelectTrigger>
              <SelectContent>
                {candidates.map((m) => (
                  <SelectItem key={m.user_id} value={m.user_id}>
                    {m.name || m.email} ({m.email})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button variant="outline" disabled={!newOwner} onClick={() => setConfirm("transfer")}>
              Transférer
            </Button>
          </CardContent>
        </Card>
      )}

      {!t.personal && (
        <Card className="border-destructive/50">
          <CardHeader>
            <CardTitle className="text-destructive">Zone de danger</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <p className="text-sm font-medium">Quitter l'équipe</p>
                <p className="text-sm text-muted-foreground">Impossible pour le dernier propriétaire.</p>
              </div>
              <Button variant="outline" onClick={() => setConfirm("leave")}>
                Quitter
              </Button>
            </div>
            {canDelete && (
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <p className="text-sm font-medium">Supprimer l'équipe</p>
                  <p className="text-sm text-muted-foreground">Avec ses sous-équipes, projets, serveurs et paramètres.</p>
                </div>
                <Button variant="destructive" onClick={() => setConfirm("delete")}>
                  Supprimer
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <ConfirmDialog
        open={confirm === "leave"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`Quitter ${t.name} ?`}
        description="Vous perdez l'accès à l'équipe et à ses sous-équipes ; un membre devra vous réinviter."
        confirmLabel="Quitter"
        destructive
        loading={leave.isPending}
        onConfirm={() => leave.mutate(undefined)}
      />
      <ConfirmDialog
        open={confirm === "transfer"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Transférer la propriété ?"
        description={`${candidates.find((m) => m.user_id === newOwner)?.email ?? ""} deviendra propriétaire de ${t.name}.`}
        confirmLabel="Transférer"
        loading={transfer.isPending}
        onConfirm={() => transfer.mutate(undefined)}
      />
      <ConfirmDialog
        open={confirm === "delete"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`Supprimer ${t.name} ?`}
        description="Action définitive."
        confirmText={t.name}
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => remove.mutate(false)}
      />
      <ConfirmDialog
        open={confirm === "delete-resources"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`${resources} ressource(s) encore présente(s)`}
        description="Les applications, bases et services de l'équipe et de ses sous-équipes seront arrêtés et effacés des serveurs, volumes compris. Les données seront perdues."
        confirmText="supprimer les ressources"
        confirmLabel="Tout supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => remove.mutate(true)}
      />
    </div>
  )
}
