import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ExternalLinkIcon, PencilIcon, RotateCwIcon } from "lucide-react"
import { CodeEditor } from "@/components/code-editor"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { TaskDialog } from "@/components/task-output"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { ApiError, patch, post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { ServerTask, Service, ServiceApp } from "@/lib/types"
import { svcBase, svcQuery } from "@/features/services/queries"

export const Route = createFileRoute("/t/$team/services/$svc/")({ component: ServiceGeneral })

function ServiceGeneral() {
  const { team, svc } = Route.useParams()
  const { data } = useQuery(svcQuery(team, svc))
  if (!data) return null
  return (
    <div className="space-y-6">
      <Apps team={team} svc={svc} apps={data.apps} />
      <ServiceForm key={data.service.updated_at} team={team} s={data.service} />
    </div>
  )
}

type Edit = { app: ServiceApp; domains: string; port: string; exclude: boolean }

function Apps({ team, svc, apps }: { team: string; svc: string; apps: ServiceApp[] }) {
  const scope = { type: "resource", id: svc }
  const canUpdate = usePermission(team, "resource.update", scope)
  const canDeploy = usePermission(team, "resource.deploy", scope)
  const base = svcBase(team, svc)
  const [edit, setEdit] = useState<Edit | null>(null)
  const [conflict, setConflict] = useState("")
  const [task, setTask] = useState<ServerTask | null>(null)
  const save = useApiMutation(
    async ({ e, force }: { e: Edit; force: boolean }) => {
      try {
        return await patch<ServiceApp>(`${base}/apps/${e.app.name}`, {
          domains: e.domains.split(/[\s,]+/).filter(Boolean),
          port: e.port,
          exclude_from_status: e.exclude,
          force_domain_override: force || undefined,
        })
      } catch (err) {
        // Domaine déjà utilisé par une autre ressource : proposer de forcer.
        if (err instanceof ApiError && err.status === 409) {
          setConflict(err.message)
          return null
        }
        throw err
      }
    },
    {
      success: (r) => (r ? "Sous-service enregistré (appliqué au prochain démarrage)" : undefined),
      invalidate: [["team", team, "svc", svc]],
      onSuccess: (r) => {
        if (!r) return
        setEdit(null)
        setConflict("")
      },
    },
  )
  const restart = useApiMutation((name: string) => post<ServerTask>(`${base}/apps/${name}/restart`), {
    onSuccess: (t) => t && "kind" in t && setTask(t),
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle>Sous-services</CardTitle>
        <CardDescription>Conteneurs du compose ; un domaine expose le sous-service via le proxy.</CardDescription>
      </CardHeader>
      <CardContent>
        <ul className="divide-y rounded-lg border">
          {apps.map((a) => (
            <li key={a.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5 text-sm">
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-2">
                  <span className="font-medium">{a.name}</span>
                  <span className="truncate font-mono text-xs text-muted-foreground">{a.image}</span>
                  {a.exclude_from_status && <Badge variant="outline">hors statut</Badge>}
                </span>
                <span className="flex flex-wrap gap-x-3 text-xs">
                  {a.domains.map((d) => (
                    <a key={d} href={d} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-primary hover:underline">
                      {d.replace(/^https?:\/\//, "")} <ExternalLinkIcon className="size-3" />
                    </a>
                  ))}
                  {!a.domains.length && <span className="text-muted-foreground">non exposé</span>}
                  {a.port && <span className="text-muted-foreground">port {a.port}</span>}
                </span>
              </span>
              {canUpdate && (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Modifier ${a.name}`}
                  onClick={() => setEdit({ app: a, domains: a.domains.join("\n"), port: a.port, exclude: a.exclude_from_status })}
                >
                  <PencilIcon />
                </Button>
              )}
              {canDeploy && (
                <Button variant="ghost" size="icon-sm" aria-label={`Redémarrer ${a.name}`} onClick={() => restart.mutate(a.name)}>
                  <RotateCwIcon />
                </Button>
              )}
            </li>
          ))}
        </ul>
      </CardContent>
      <Dialog open={!!edit} onOpenChange={(o) => !o && setEdit(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{edit?.app.name}</DialogTitle>
            <DialogDescription>Pris en compte au prochain démarrage du service.</DialogDescription>
          </DialogHeader>
          {edit && (
            <div className="space-y-4">
              <Field label="Domaines" hint="Un par ligne ; un port peut être précisé (https://app.example.com:3000).">
                <Textarea rows={3} className="font-mono text-xs" value={edit.domains} onChange={(e) => setEdit({ ...edit, domains: e.target.value })} />
              </Field>
              <Field label="Port du conteneur">
                <Input className="font-mono" value={edit.port} onChange={(e) => setEdit({ ...edit, port: e.target.value })} />
              </Field>
              <Label className="flex items-center justify-between font-normal">
                Ignorer ce conteneur dans le statut du service
                <Switch checked={edit.exclude} onCheckedChange={(v) => setEdit({ ...edit, exclude: v })} />
              </Label>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEdit(null)}>
              Annuler
            </Button>
            <Button loading={save.isPending} onClick={() => edit && save.mutate({ e: edit, force: false })}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!conflict}
        onOpenChange={(o) => !o && setConflict("")}
        title="Domaine déjà utilisé"
        description={`${conflict}. Enregistrer quand même ?`}
        confirmLabel="Forcer"
        destructive
        loading={save.isPending}
        onConfirm={() => edit && save.mutate({ e: edit, force: true })}
      />
      <TaskDialog team={team} task={task} onOpenChange={(o) => !o && setTask(null)} />
    </Card>
  )
}

function ServiceForm({ team, s }: { team: string; s: Service }) {
  const canUpdate = usePermission(team, "resource.update", { type: "resource", id: s.id })
  const [f, setF] = useState({ name: s.name, description: s.description, docker_compose_raw: s.docker_compose_raw })
  const save = useApiMutation(() => patch(svcBase(team, s.id), f), {
    success: "Service enregistré (appliqué au prochain démarrage)",
    invalidate: [["team", team, "svc", s.id]],
  })
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(undefined)
      }}
    >
      <Card>
        <CardHeader>
          <CardTitle>Configuration</CardTitle>
          <CardDescription>
            Un nouveau compose est ré-analysé : nouveaux secrets générés, valeurs et domaines existants conservés. Les variables sont dans{" "}
            <Link to="/t/$team/services/$svc/envs" params={{ team, svc: s.id }} className="underline">
              Variables
            </Link>
            .
          </CardDescription>
        </CardHeader>
        <CardContent>
          <fieldset disabled={!canUpdate} className="grid gap-4 md:grid-cols-2">
            <Field label="Nom">
              <Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
            </Field>
            <Field label="Description">
              <Input value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
            </Field>
            <Field label="docker-compose.yml" className="md:col-span-2">
              <CodeEditor language="yaml" value={f.docker_compose_raw} onChange={(v) => setF({ ...f, docker_compose_raw: v })} readOnly={!canUpdate} minHeight="320px" />
            </Field>
          </fieldset>
          {canUpdate && (
            <div className="mt-4 flex justify-end">
              <Button type="submit" loading={save.isPending}>
                Enregistrer
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </form>
  )
}
