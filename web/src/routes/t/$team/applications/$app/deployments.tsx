import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ArrowLeftIcon, BanIcon, GitCommitIcon, HistoryIcon, RocketIcon, Undo2Icon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { get, post } from "@/lib/api"
import { dateTime, duration, relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Deployment, ImageInfo } from "@/lib/types"
import { appBase, deploymentsQuery, imagesQuery } from "@/features/applications/queries"
import { DeploymentLogs } from "@/features/applications/deployment-logs"

export const Route = createFileRoute("/t/$team/applications/$app/deployments")({
  validateSearch: (s: Record<string, unknown>): { dep?: string } => (typeof s.dep === "string" ? { dep: s.dep } : {}),
  component: AppDeployments,
})

const TRIGGER: Record<string, string> = {
  manual: "Manuel",
  api: "API",
  webhook: "Webhook",
  rollback: "Retour arrière",
  restart: "Redémarrage",
  approval: "Après approbation",
  preview: "Preview",
}

function AppDeployments() {
  const { team, app } = Route.useParams()
  const { dep } = Route.useSearch()
  if (dep) return <DeploymentDetail team={team} app={app} id={dep} />
  return (
    <div className="space-y-6">
      <DeploymentList team={team} app={app} />
      <Rollback team={team} app={app} />
    </div>
  )
}

function DeploymentList({ team, app }: { team: string; app: string }) {
  const deps = useQuery(deploymentsQuery(team, app))
  if (deps.isPending) return <Skeleton className="h-48" />
  if (!deps.data?.length) return <EmptyState icon={RocketIcon} title="Aucun déploiement" description="Lancez le premier avec le bouton « Déployer »." />
  return (
    <Card>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Statut</TableHead>
            <TableHead>Commit</TableHead>
            <TableHead>Origine</TableHead>
            <TableHead>Lancé</TableHead>
            <TableHead className="text-right">Durée</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {deps.data.map((d) => (
            <TableRow key={d.id} className="cursor-pointer">
              <TableCell>
                <Link to="/t/$team/applications/$app/deployments" params={{ team, app }} search={{ dep: d.id }} className="block">
                  <StatusBadge status={d.status} />
                </Link>
              </TableCell>
              <TableCell className="max-w-80">
                <Link to="/t/$team/applications/$app/deployments" params={{ team, app }} search={{ dep: d.id }} className="block truncate">
                  <Commit d={d} />
                </Link>
              </TableCell>
              <TableCell>
                <Badge variant="outline">{d.pr_number ? `PR #${d.pr_number}` : (TRIGGER[d.trigger] ?? d.trigger)}</Badge>
                {d.force_rebuild && <Badge variant="outline">sans cache</Badge>}
              </TableCell>
              <TableCell className="text-muted-foreground" title={dateTime(d.created_at)}>
                {relativeTime(d.created_at)}
              </TableCell>
              <TableCell className="text-right text-muted-foreground tabular-nums">{d.started_at ? duration(d.started_at, d.finished_at) : "—"}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

function Commit({ d }: { d: Deployment }) {
  if (d.rollback_image) return <span className="font-mono text-xs">{d.rollback_image}</span>
  return (
    <span className="inline-flex min-w-0 items-center gap-1.5">
      <GitCommitIcon className="size-3.5 shrink-0 text-muted-foreground" />
      <span className="font-mono text-xs">{d.commit_sha && d.commit_sha !== "HEAD" ? d.commit_sha.slice(0, 7) : (d.branch || "HEAD")}</span>
      {d.commit_message && <span className="truncate text-muted-foreground">{d.commit_message.split("\n")[0]}</span>}
    </span>
  )
}

function DeploymentDetail({ team, app, id }: { team: string; app: string; id: string }) {
  const base = appBase(team, app)
  const canDeploy = usePermission(team, "resource.deploy", { type: "resource", id: app })
  const dep = useQuery({ queryKey: ["team", team, "app", app, "deployments", id], queryFn: () => get<Deployment>(`${base}/deployments/${id}`) })
  const [live, setLive] = useState<string>()
  const cancel = useApiMutation(() => post(`${base}/deployments/${id}/cancel`), {
    success: "Annulation demandée",
    invalidate: [["team", team, "app", app]],
  })
  const d = dep.data
  const status = live ?? d?.status
  return (
    <div className="space-y-4">
      <Button variant="ghost" size="sm" asChild>
        <Link to="/t/$team/applications/$app/deployments" params={{ team, app }}>
          <ArrowLeftIcon /> Tous les déploiements
        </Link>
      </Button>
      {d && (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-x-6 gap-y-2 pt-5 text-sm">
            <Commit d={d} />
            <span className="text-muted-foreground">{d.pr_number ? `PR #${d.pr_number}` : (TRIGGER[d.trigger] ?? d.trigger)}</span>
            <span className="text-muted-foreground">Lancé {relativeTime(d.created_at)}</span>
            {d.started_at && <span className="text-muted-foreground">Durée {duration(d.started_at, d.finished_at)}</span>}
            {d.image && <span className="font-mono text-xs text-muted-foreground">{d.image}</span>}
            {canDeploy && (status === "queued" || status === "in_progress") && (
              <Button size="sm" variant="outline" className="ml-auto" loading={cancel.isPending} onClick={() => cancel.mutate(undefined)}>
                <BanIcon /> Annuler
              </Button>
            )}
          </CardContent>
        </Card>
      )}
      <DeploymentLogs base={base} deployment={id} onStatus={setLive} />
    </div>
  )
}

function Rollback({ team, app }: { team: string; app: string }) {
  const canDeploy = usePermission(team, "resource.deploy", { type: "resource", id: app })
  const images = useQuery(imagesQuery(team, app))
  const navigate = Route.useNavigate()
  const [target, setTarget] = useState<ImageInfo | null>(null)
  const rollback = useApiMutation((tag: string) => post<Deployment & { approval_request?: unknown }>(`${appBase(team, app)}/rollback`, { tag }), {
    success: (r) => (r && "id" in r && !r.approval_request ? "Retour arrière lancé" : undefined),
    invalidate: [["team", team, "app", app]],
    onSuccess: (r) => {
      setTarget(null)
      if (r && "id" in r && r.id && !r.approval_request) void navigate({ search: { dep: r.id } })
    },
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <HistoryIcon className="size-4" /> Retour arrière
        </CardTitle>
        <CardDescription>Images construites encore présentes sur le serveur (nombre réglable dans « Avancé »).</CardDescription>
      </CardHeader>
      <CardContent>
        {images.isError ? (
          <p className="text-sm text-muted-foreground">Serveur injoignable : impossible de lister les images.</p>
        ) : !images.data?.length ? (
          <p className="text-sm text-muted-foreground">Aucune image disponible.</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {images.data.map((i) => (
              <li key={i.tag} className="flex items-center gap-3 px-3 py-2 text-sm">
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-mono text-xs">{i.tag}</span>
                  <span className="text-xs text-muted-foreground">
                    {i.created ? relativeTime(i.created) : ""} · {i.size}
                  </span>
                </span>
                {i.current ? (
                  <Badge variant="success">En service</Badge>
                ) : (
                  canDeploy && (
                    <Button size="sm" variant="outline" onClick={() => setTarget(i)}>
                      <Undo2Icon /> Revenir
                    </Button>
                  )
                )}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
      <ConfirmDialog
        open={!!target}
        onOpenChange={(o) => !o && setTarget(null)}
        title="Revenir à cette image ?"
        description={<span className="font-mono text-xs">{target?.tag}</span>}
        confirmLabel="Revenir"
        loading={rollback.isPending}
        onConfirm={() => target && rollback.mutate(target.tag)}
      />
    </Card>
  )
}
