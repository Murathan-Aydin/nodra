import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { api, post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { useDestinations, useEnvironments } from "./locations"

type Kind = "application" | "database" | "service"

/** Déplacement, clonage et suppression d'une ressource. */
export function ResourceActions({
  team,
  kind,
  id,
  name,
  base,
  environmentId,
  destinationId,
}: {
  team: string
  kind: Kind
  id: string
  name: string
  base: string
  environmentId: string
  destinationId: string
}) {
  const navigate = useNavigate()
  const scope = { type: "resource", id }
  const canUpdate = usePermission(team, "resource.update", scope)
  const canDelete = usePermission(team, "resource.delete", scope)
  const envs = useEnvironments(team)
  const dests = useDestinations(team)
  const [moveTo, setMoveTo] = useState("")
  const [clone, setClone] = useState<{ name: string; environment_id: string; destination_id: string } | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [volumes, setVolumes] = useState(false)
  const move = useApiMutation((env: string) => post(`${base}/move`, { environment_id: env }), {
    success: "Ressource déplacée",
    invalidate: [["team", team]],
    onSuccess: () => setMoveTo(""),
  })
  const doClone = useApiMutation((c: NonNullable<typeof clone>) => post<{ id: string }>(`${base}/clone`, c), {
    success: "Ressource clonée (arrêtée)",
    invalidate: [["team", team, "resources"]],
    onSuccess: (r) => {
      setClone(null)
      if (kind === "application") void navigate({ to: "/t/$team/applications/$app", params: { team, app: r.id } })
      if (kind === "database") void navigate({ to: "/t/$team/databases/$db", params: { team, db: r.id } })
      if (kind === "service") void navigate({ to: "/t/$team/services/$svc", params: { team, svc: r.id } })
    },
  })
  const remove = useApiMutation(() => api(base, { method: "DELETE", query: { delete_volumes: volumes || undefined } }), {
    success: (r) => (r ? undefined : "Ressource supprimée"),
    invalidate: [["team", team, "resources"]],
    onSuccess: (r) => {
      setDeleting(false)
      if (!r) void navigate({ to: "/t/$team", params: { team } })
    },
  })
  const envLabel = (e: (typeof envs)[number]) => `${e.project.name} / ${e.name}`
  return (
    <div className="space-y-6">
      {canUpdate && (
        <Card>
          <CardHeader>
            <CardTitle>Déplacer et cloner</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-end gap-2">
              <Field label="Déplacer vers un autre environnement" className="min-w-72 flex-1">
                <Select value={moveTo} onValueChange={setMoveTo}>
                  <SelectTrigger>
                    <SelectValue placeholder="Choisir un environnement" />
                  </SelectTrigger>
                  <SelectContent>
                    {envs
                      .filter((e) => e.id !== environmentId)
                      .map((e) => (
                        <SelectItem key={e.id} value={e.id}>
                          {envLabel(e)}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </Field>
              <Button variant="outline" disabled={!moveTo} loading={move.isPending} onClick={() => move.mutate(moveTo)}>
                Déplacer
              </Button>
            </div>
            <div className="flex items-center justify-between gap-4 border-t pt-4">
              <div>
                <p className="text-sm font-medium">Cloner</p>
                <p className="text-xs text-muted-foreground">Copie la configuration, les variables et les stockages (sans les données). La copie est arrêtée, avec de nouveaux domaines.</p>
              </div>
              <Button variant="outline" onClick={() => setClone({ name: `${name} (copie)`, environment_id: environmentId, destination_id: destinationId })}>
                Cloner
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {canDelete && (
        <Card className="border-destructive/40">
          <CardHeader>
            <CardTitle className="text-destructive">Zone de danger</CardTitle>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium">Supprimer la ressource</p>
              <p className="text-xs text-muted-foreground">Arrête les conteneurs et supprime la configuration. Peut exiger une approbation.</p>
            </div>
            <Button variant="destructive" onClick={() => setDeleting(true)}>
              Supprimer
            </Button>
          </CardContent>
        </Card>
      )}
      <Dialog open={!!clone} onOpenChange={(o) => !o && setClone(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cloner {name}</DialogTitle>
            <DialogDescription>Les données (volumes) ne sont pas copiées.</DialogDescription>
          </DialogHeader>
          {clone && (
            <div className="space-y-4">
              <Field label="Nom">
                <Input value={clone.name} onChange={(e) => setClone({ ...clone, name: e.target.value })} />
              </Field>
              <Field label="Environnement">
                <Select value={clone.environment_id} onValueChange={(v) => setClone({ ...clone, environment_id: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {envs.map((e) => (
                      <SelectItem key={e.id} value={e.id}>
                        {envLabel(e)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Serveur / réseau">
                <Select value={clone.destination_id} onValueChange={(v) => setClone({ ...clone, destination_id: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {dests.map((d) => (
                      <SelectItem key={d.id} value={d.id}>
                        {d.server.name} · {d.network}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setClone(null)}>
              Annuler
            </Button>
            <Button loading={doClone.isPending} onClick={() => clone && doClone.mutate(clone)}>
              Cloner
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={deleting}
        onOpenChange={setDeleting}
        title={`Supprimer ${name} ?`}
        description="Les conteneurs sont arrêtés et supprimés, ainsi que la configuration (variables, stockages, tags)."
        confirmText={name}
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => remove.mutate(undefined)}
      >
        <Label className="font-normal">
          <Checkbox checked={volumes} onCheckedChange={(c) => setVolumes(c === true)} /> Supprimer aussi les volumes Docker (données)
        </Label>
      </ConfirmDialog>
    </div>
  )
}
