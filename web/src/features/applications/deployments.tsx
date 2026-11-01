import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useEffect, useRef, useState } from "react"
import { RotateCcwIcon, XIcon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { get, post } from "@/lib/api"
import { bytes, dateTime, duration, relativeTime } from "@/lib/format"
import { applicationQueries } from "@/lib/queries"
import { useSocket } from "@/lib/socket"
import type { Deployment, DeploymentLog } from "@/lib/types"
import { resourceApi, resourceKey, resourcePage, type ResourceRef } from "@/features/resources/resource"

export const DEPLOY_STATUS: Record<Deployment["status"], { label: string; dot: string }> = {
  queued: { label: "En attente", dot: "bg-muted-foreground" },
  in_progress: { label: "En cours", dot: "bg-primary animate-pulse" },
  finished: { label: "Terminé", dot: "bg-success" },
  failed: { label: "Échoué", dot: "bg-destructive" },
  cancelled: { label: "Annulé", dot: "bg-warning" },
}

const TRIGGER: Record<string, string> = { manual: "Manuel", api: "API", webhook: "Webhook", rollback: "Retour arrière", restart: "Redémarrage", approval: "Approbation", preview: "Preview" }

/** Onglet « Déploiements » : historique, puis retour arrière vers une image précédente. */
export function DeploymentsTab({ resource }: { resource: ResourceRef }) {
  const deployments = useQuery({ ...applicationQueries.deployments(resource.team, resource.id), refetchInterval: 10_000 })
  return (
    <div className="space-y-6">
      <SettingsCard title="Déploiements" description="50 derniers déploiements." bodyClassName="p-0">
        {deployments.isPending ? (
          <Skeleton className="m-4 h-24" />
        ) : deployments.data?.length ? (
          deployments.data.map((d) => {
            const s = DEPLOY_STATUS[d.status]
            return (
              <Link key={d.id} to={resourcePage(resource, `/deployments/${d.id}`)} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0 hover:bg-foreground/[0.03]">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    {d.commit_message?.split("\n")[0] || (d.commit_sha ? d.commit_sha.slice(0, 7) : "Déploiement")}
                    {d.pr_number ? <span className="ml-2 text-xs text-muted-foreground">PR #{d.pr_number}</span> : null}
                  </p>
                  <p className="truncate text-xs text-muted-foreground" title={dateTime(d.created_at)}>
                    {TRIGGER[d.trigger ?? ""] ?? d.trigger} · {d.branch} {d.commit_sha && `· ${d.commit_sha.slice(0, 7)}`} · {relativeTime(d.created_at)}
                    {d.started_at && ` · ${duration(d.started_at, d.finished_at)}`}
                  </p>
                </div>
                <Pill label={s.label} dot={s.dot} />
              </Link>
            )
          })
        ) : (
          <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucun déploiement. Utilisez « Déployer » en haut à droite.</p>
        )}
      </SettingsCard>
      <Rollback resource={resource} />
    </div>
  )
}

function Rollback({ resource }: { resource: ResourceRef }) {
  const navigate = useNavigate()
  const images = useQuery({ queryKey: resourceKey(resource, "images"), queryFn: () => get<{ tag: string; image: string; created: string; size: number; current: boolean }[]>(resourceApi(resource, "/images")) })
  const [tag, setTag] = useState<string | null>(null)
  const rollback = useMutation({
    mutationFn: (t: string) => post<Deployment & { approval_request?: unknown }>(resourceApi(resource, "/rollback"), { tag: t }),
    onSuccess: (r) => {
      setTag(null)
      if (r?.approval_request) return void toast.info("Retour arrière en attente d'approbation")
      toast.success("Retour arrière lancé")
      if (r?.id) void navigate({ to: resourcePage(resource, `/deployments/${r.id}`) })
    },
    onError: (e) => toast.error(e.message),
  })
  if (!images.data?.length) return null
  return (
    <SettingsCard title="Retour arrière" description="Images gardées sur le serveur (réglage « Images gardées » dans Avancé)." bodyClassName="p-0">
      {images.data.map((i) => (
        <div key={i.tag} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
          <div className="min-w-0 flex-1">
            <p className="truncate font-mono text-sm">{i.tag}</p>
            <p className="text-xs text-muted-foreground">
              {relativeTime(i.created)} · {bytes(i.size)}
            </p>
          </div>
          {i.current ? (
            <Pill label="Actuelle" dot="bg-success" />
          ) : (
            <Button size="sm" variant="outline" onClick={() => setTag(i.tag)}>
              <RotateCcwIcon /> Revenir à cette version
            </Button>
          )}
        </div>
      ))}
      <ConfirmDialog open={!!tag} onOpenChange={(o) => !o && setTag(null)} title="Revenir à cette version ?" description={tag ?? undefined} destructive={false} actionLabel="Revenir" loading={rollback.isPending} onConfirm={() => tag && rollback.mutate(tag)} />
    </SettingsCard>
  )
}

/** Détail d'un déploiement : statut, logs en direct (WebSocket) et annulation. */
export function DeploymentDetail({ resource, deploymentId }: { resource: ResourceRef; deploymentId: string }) {
  const qc = useQueryClient()
  const dep = useQuery({ queryKey: resourceKey(resource, "deployment", deploymentId), queryFn: () => get<Deployment>(resourceApi(resource, `/deployments/${deploymentId}`)) })
  const [debug, setDebug] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const cancel = useMutation({
    mutationFn: () => post(resourceApi(resource, `/deployments/${deploymentId}/cancel`)),
    onSuccess: () => {
      setCancelling(false)
      toast.success("Annulation demandée")
      void qc.invalidateQueries({ queryKey: resourceKey(resource, "deployment", deploymentId) })
    },
    onError: (e) => toast.error(e.message),
  })
  const d = dep.data
  const s = d ? DEPLOY_STATUS[d.status] : null
  const active = d && (d.status === "queued" || d.status === "in_progress")
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Déploiement"
        description={d ? `${TRIGGER[d.trigger ?? ""] ?? ""} · ${d.branch ?? ""} ${d.commit_sha ? `· ${d.commit_sha.slice(0, 7)}` : ""} · ${dateTime(d.created_at)}${d.started_at ? ` · ${duration(d.started_at, d.finished_at)}` : ""}` : undefined}
        actions={
          <>
            {s && <Pill label={s.label} dot={s.dot} />}
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <Switch checked={debug} onCheckedChange={setDebug} /> Logs détaillés
            </label>
            {active && (
              <Button size="sm" variant="outline" disabled={d.cancel_requested} onClick={() => setCancelling(true)}>
                <XIcon /> {d.cancel_requested ? "Annulation…" : "Annuler"}
              </Button>
            )}
          </>
        }
        bodyClassName="p-0"
      >
        {d?.commit_message && <p className="border-b border-border px-4 py-2 text-sm">{d.commit_message.split("\n")[0]}</p>}
        <DeploymentLogs key={`${deploymentId}-${debug}`} resource={resource} deploymentId={deploymentId} debug={debug} onStatus={() => void qc.invalidateQueries({ queryKey: resourceKey(resource, "deployment", deploymentId) })} />
        {d?.error && <p className="border-t border-border px-4 py-3 text-sm text-destructive">{d.error}</p>}
      </SettingsCard>
      <ConfirmDialog open={cancelling} onOpenChange={setCancelling} title="Annuler ce déploiement ?" description="La version en cours reste en place." actionLabel="Annuler le déploiement" loading={cancel.isPending} onConfirm={() => cancel.mutate()} />
    </div>
  )
}

function DeploymentLogs({ resource, deploymentId, debug, onStatus }: { resource: ResourceRef; deploymentId: string; debug: boolean; onStatus: () => void }) {
  const [logs, setLogs] = useState<DeploymentLog[]>([])
  const box = useRef<HTMLDivElement>(null)
  const follow = useRef(true)
  useSocket(resourceApi(resource, `/deployments/${deploymentId}/logs/stream${debug ? "?debug=1" : ""}`), {
    onText: (data) => {
      try {
        const msg = JSON.parse(data) as { type: string; logs?: DeploymentLog[] }
        if (msg.type === "logs" && msg.logs) setLogs((l) => [...l, ...msg.logs!.filter((x) => !l.some((y) => y.id === x.id))])
        if (msg.type === "status" || msg.type === "end") onStatus()
      } catch {
        // message inattendu : ignoré
      }
    },
  })
  useEffect(() => {
    if (follow.current && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [logs])
  return (
    <div
      ref={box}
      onScroll={(e) => {
        const el = e.currentTarget
        follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
      }}
      className="h-[65vh] overflow-auto px-4 py-3 font-mono text-xs leading-5"
    >
      {logs.map((l) => (
        <div key={l.id} className={l.kind === "stderr" ? "text-destructive" : l.hidden ? "text-muted-foreground" : undefined}>
          <span className="mr-3 text-muted-foreground select-none">{new Date(l.created_at).toLocaleTimeString("fr")}</span>
          <span className="whitespace-pre-wrap">{l.message}</span>
        </div>
      ))}
      {logs.length === 0 && <span className="text-muted-foreground">En attente des logs…</span>}
    </div>
  )
}
