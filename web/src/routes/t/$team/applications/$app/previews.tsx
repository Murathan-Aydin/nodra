import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ExternalLinkIcon, GitPullRequestIcon, PencilIcon, PlusIcon, RocketIcon, SquareIcon, Trash2Icon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { del, patch, post } from "@/lib/api"
import { relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Deployment, Preview } from "@/lib/types"
import { appBase, appQuery, previewsQuery } from "@/features/applications/queries"

export const Route = createFileRoute("/t/$team/applications/$app/previews")({ component: AppPreviews })

const parseList = (s: string) => s.split(/[\s,]+/).filter(Boolean)

function AppPreviews() {
  const { team, app } = Route.useParams()
  const navigate = Route.useNavigate()
  const base = appBase(team, app)
  const scope = { type: "resource", id: app }
  const canDeploy = usePermission(team, "resource.deploy", scope)
  const canUpdate = usePermission(team, "resource.update", scope)
  const application = useQuery(appQuery(team, app)).data?.application
  const previews = useQuery(previewsQuery(team, app))
  const [creating, setCreating] = useState<{ pr_number: string; title: string } | null>(null)
  const [editing, setEditing] = useState<{ pr: number; domains: string } | null>(null)
  const [deleting, setDeleting] = useState<Preview | null>(null)
  const key = ["team", team, "app", app]
  const toDeployment = (r: (Deployment & { approval_request?: unknown }) | undefined) => {
    if (r && "id" in r && r.id && !r.approval_request) void navigate({ to: "/t/$team/applications/$app/deployments", search: { dep: r.id } })
  }
  const toggle = useApiMutation((v: boolean) => patch(base, { settings: { preview_deployments: v } }), {
    success: (_, v) => (v ? "Previews automatiques activées" : "Previews automatiques désactivées"),
    invalidate: [key],
  })
  const create = useApiMutation(
    (c: NonNullable<typeof creating>) => post<Deployment & { approval_request?: unknown }>(`${base}/previews`, { pr_number: Number(c.pr_number), title: c.title }),
    { success: "Preview créée", invalidate: [key], onSuccess: toDeployment },
  )
  const deploy = useApiMutation((pr: number) => post<Deployment & { approval_request?: unknown }>(`${base}/previews/${pr}/deploy`, {}), {
    success: "Déploiement de la preview lancé",
    invalidate: [key],
    onSuccess: toDeployment,
  })
  const stop = useApiMutation((pr: number) => post(`${base}/previews/${pr}/stop`), { success: "Preview arrêtée", invalidate: [key] })
  const save = useApiMutation(
    (e: NonNullable<typeof editing>) =>
      patch(
        `${base}/previews/${e.pr}`,
        application?.build_pack === "dockercompose"
          ? {
              compose_domains: Object.fromEntries(
                e.domains
                  .split("\n")
                  .filter((l) => l.includes("="))
                  .map((l) => {
                    const [svc, ds] = l.split("=")
                    return [svc.trim(), parseList(ds)]
                  }),
              ),
            }
          : { domains: parseList(e.domains) },
      ),
    { success: "Domaines enregistrés (appliqués au prochain déploiement)", invalidate: [key], onSuccess: () => setEditing(null) },
  )
  const remove = useApiMutation((pr: number) => del(`${base}/previews/${pr}`), {
    success: "Preview supprimée",
    invalidate: [key],
    onSuccess: () => setDeleting(null),
  })
  const compose = application?.build_pack === "dockercompose"

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-3 pt-5">
          <Label className="font-normal">
            <Switch
              checked={!!application?.settings.preview_deployments}
              onCheckedChange={(v) => toggle.mutate(v)}
              disabled={!canUpdate || !application || toggle.isPending}
            />
            Déployer automatiquement chaque pull request
          </Label>
          <p className="text-sm text-muted-foreground">
            URL : <span className="font-mono">{application?.preview_url_template}</span> (modifiable dans{" "}
            <Link to="/t/$team/applications/$app" params={{ team, app }} className="underline">
              Général
            </Link>
            )
          </p>
        </CardContent>
      </Card>
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">Une preview par pull request, avec ses propres variables (onglet « Variables »).</p>
        {canDeploy && (
          <Button size="sm" onClick={() => setCreating({ pr_number: "", title: "" })}>
            <PlusIcon /> Preview manuelle
          </Button>
        )}
      </div>
      {!previews.data?.length ? (
        <EmptyState icon={GitPullRequestIcon} title="Aucune preview" description="Elles apparaissent à l'ouverture d'une pull request si les previews automatiques sont actives." />
      ) : (
        <Card>
          <ul className="divide-y">
            {previews.data.map((p) => {
              const domains = compose ? Object.values(p.compose_domains ?? {}).flat() : p.domains
              return (
                <li key={p.id} className="flex flex-wrap items-center gap-3 px-4 py-3 text-sm">
                  <GitPullRequestIcon className="size-4 text-muted-foreground" />
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                      <span className="font-medium">PR #{p.pr_number}</span>
                      {p.title && <span className="truncate text-muted-foreground">{p.title}</span>}
                      {p.pr_url && (
                        <a href={p.pr_url} target="_blank" rel="noreferrer" aria-label="Ouvrir la pull request" className="text-muted-foreground hover:text-foreground">
                          <ExternalLinkIcon className="size-3.5" />
                        </a>
                      )}
                    </span>
                    <span className="flex flex-wrap gap-x-3 text-xs text-muted-foreground">
                      {domains.map((d) => (
                        <a key={d} href={d} target="_blank" rel="noreferrer" className="hover:underline">
                          {d.replace(/^https?:\/\//, "")}
                        </a>
                      ))}
                      {p.last_commit && <span className="font-mono">{p.last_commit.slice(0, 7)}</span>}
                      <span>mise à jour {relativeTime(p.updated_at)}</span>
                    </span>
                  </span>
                  <StatusBadge status={p.status} />
                  {canUpdate && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label="Domaines"
                      onClick={() =>
                        setEditing({
                          pr: p.pr_number,
                          domains: compose
                            ? Object.entries(p.compose_domains ?? {})
                                .map(([s, ds]) => `${s}=${ds.join(",")}`)
                                .join("\n")
                            : p.domains.join("\n"),
                        })
                      }
                    >
                      <PencilIcon />
                    </Button>
                  )}
                  {canDeploy && (
                    <>
                      <Button variant="ghost" size="icon-sm" aria-label="Redéployer" onClick={() => deploy.mutate(p.pr_number)}>
                        <RocketIcon />
                      </Button>
                      <Button variant="ghost" size="icon-sm" aria-label="Arrêter" onClick={() => stop.mutate(p.pr_number)}>
                        <SquareIcon />
                      </Button>
                      <Button variant="ghost" size="icon-sm" aria-label="Supprimer" onClick={() => setDeleting(p)}>
                        <Trash2Icon />
                      </Button>
                    </>
                  )}
                </li>
              )
            })}
          </ul>
        </Card>
      )}

      <Dialog open={!!creating} onOpenChange={(o) => !o && setCreating(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Preview manuelle</DialogTitle>
            <DialogDescription>Déploie la branche d'une pull request existante.</DialogDescription>
          </DialogHeader>
          {creating && (
            <div className="space-y-4">
              <Field label="Numéro de la pull request">
                <Input type="number" min={1} value={creating.pr_number} onChange={(e) => setCreating({ ...creating, pr_number: e.target.value })} />
              </Field>
              <Field label="Titre (facultatif)">
                <Input value={creating.title} onChange={(e) => setCreating({ ...creating, title: e.target.value })} />
              </Field>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreating(null)}>
              Annuler
            </Button>
            <Button loading={create.isPending} disabled={!Number(creating?.pr_number)} onClick={() => creating && create.mutate(creating)}>
              Déployer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!editing} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Domaines de la PR #{editing?.pr}</DialogTitle>
            <DialogDescription>{compose ? "Une ligne par service : web=https://pr-12.example.com" : "Un domaine par ligne."}</DialogDescription>
          </DialogHeader>
          {editing && <Textarea rows={4} className="font-mono text-xs" value={editing.domains} onChange={(e) => setEditing({ ...editing, domains: e.target.value })} />}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)}>
              Annuler
            </Button>
            <Button loading={save.isPending} onClick={() => editing && save.mutate(editing)}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Supprimer la preview de la PR #${deleting?.pr_number} ?`}
        description="Les déploiements en cours sont annulés et les conteneurs supprimés."
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.pr_number)}
      />
    </div>
  )
}
