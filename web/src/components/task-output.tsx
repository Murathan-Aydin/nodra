import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef, useState } from "react"
import { SquareIcon } from "lucide-react"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { get, post, teamPath } from "@/lib/api"
import { stripAnsi } from "@/lib/ansi"
import { duration } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import type { ServerTask } from "@/lib/types"
import { useSocket } from "@/lib/ws"
import { cn } from "@/lib/utils"

const KINDS: Record<string, string> = {
  validate: "Validation du serveur",
  install_docker: "Installation de Docker",
  docker_cleanup: "Nettoyage Docker",
  packages_check: "Recherche de mises à jour",
  packages_update: "Mise à jour des paquets",
  reboot: "Redémarrage du serveur",
  command: "Commande",
  cloudflared_start: "Démarrage de Cloudflare Tunnel",
  cloudflared_stop: "Arrêt de Cloudflare Tunnel",
  network_create: "Création d'un réseau",
  network_delete: "Suppression d'un réseau",
  proxy_start: "Démarrage du proxy",
  proxy_stop: "Arrêt du proxy",
  proxy_restart: "Redémarrage du proxy",
  proxy_sync: "Synchronisation du proxy",
  database_start: "Démarrage de la base",
  database_stop: "Arrêt de la base",
  database_restart: "Redémarrage de la base",
  database_proxy: "Exposition de la base",
  service_start: "Démarrage du service",
  service_stop: "Arrêt du service",
  service_restart: "Redémarrage du service",
  service_restart_app: "Redémarrage d'un conteneur",
  backup_run: "Sauvegarde",
  backup_restore: "Restauration",
  database_import: "Import de dump",
  scheduled_task: "Tâche planifiée",
  log_drain_start: "Démarrage du log drain",
  log_drain_stop: "Arrêt du log drain",
}

export function taskLabel(kind: string): string {
  return KINDS[kind] ?? kind
}

const ACTIVE = new Set(["queued", "running"])

/** Sortie d'une tâche serveur en direct (WebSocket), avec annulation. */
export function TaskOutput({ team, server, task, className }: { team: string; server: string; task: string; className?: string }) {
  const qc = useQueryClient()
  const base = teamPath(team, `/servers/${server}/tasks/${task}`)
  const initial = useQuery({ queryKey: ["team", team, "task", task], queryFn: () => get<ServerTask>(base) })
  const [output, setOutput] = useState("")
  const [status, setStatus] = useState<string>()
  const [error, setError] = useState("")
  const pre = useRef<HTMLPreElement>(null)
  const stick = useRef(true)

  useSocket(base + "/stream", undefined, {
    onMessage: (m) => {
      if (m.type === "reset") setOutput("")
      if (m.type === "output") setOutput((o) => o + stripAnsi(String(m.data)))
      if (m.type === "status" || m.type === "end") setStatus(String(m.status))
      if (m.type === "end") {
        setError(String(m.error ?? ""))
        void qc.invalidateQueries({ queryKey: ["team", team, "servers"] })
        void qc.invalidateQueries({ queryKey: ["team", team, "server", server] })
      }
    },
  })

  useEffect(() => {
    const el = pre.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [output])

  const cancel = useApiMutation(() => post<{ status?: string }>(base + "/cancel"), {
    success: (r) => (r?.status === "cancelling" ? "Arrêt demandé" : "Tâche annulée"),
  })
  const current = status ?? initial.data?.status ?? "queued"
  const t = initial.data

  return (
    <div className={cn("space-y-2", className)}>
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm">
          <StatusBadge status={current === "running" ? "in_progress" : current === "success" ? "success" : current} />
          {t && <span className="text-muted-foreground">{duration(t.started_at ?? t.created_at, ACTIVE.has(current) ? null : t.finished_at)}</span>}
        </div>
        {ACTIVE.has(current) && (
          <Button size="sm" variant="outline" loading={cancel.isPending} onClick={() => cancel.mutate(undefined)}>
            <SquareIcon /> Annuler
          </Button>
        )}
      </div>
      <pre
        ref={pre}
        onScroll={(e) => {
          const el = e.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
        }}
        className="max-h-[55vh] min-h-40 overflow-auto rounded-lg bg-zinc-950 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-zinc-100"
      >
        {output || <span className="text-zinc-500">{ACTIVE.has(current) ? "En attente de la sortie…" : "Aucune sortie."}</span>}
      </pre>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  )
}

/** Fenêtre de suivi d'une tâche (après une action qui renvoie `202 ServerTask`). */
export function TaskDialog({
  team,
  task,
  onOpenChange,
}: {
  team: string
  task: Pick<ServerTask, "id" | "server_id" | "kind"> | null
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={!!task} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{task ? taskLabel(task.kind) : ""}</DialogTitle>
          <DialogDescription>Sortie en direct de la tâche sur le serveur.</DialogDescription>
        </DialogHeader>
        {task && <TaskOutput team={team} server={task.server_id} task={task.id} />}
      </DialogContent>
    </Dialog>
  )
}
