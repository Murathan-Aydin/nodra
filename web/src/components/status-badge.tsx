import { Badge } from "@/components/ui/badge"

type Variant = "success" | "warning" | "destructive" | "secondary" | "info" | "outline"

const STATUS: Record<string, [string, Variant]> = {
  running: ["En cours d'exécution", "success"],
  healthy: ["Sain", "success"],
  stopped: ["Arrêté", "secondary"],
  exited: ["Arrêté", "secondary"],
  restarting: ["Redémarrage", "warning"],
  degraded: ["Dégradé", "warning"],
  unhealthy: ["En mauvaise santé", "warning"],
  starting: ["Démarrage", "info"],
  queued: ["En file", "secondary"],
  in_progress: ["En cours", "info"],
  running_task: ["En cours", "info"],
  finished: ["Terminé", "success"],
  success: ["Réussi", "success"],
  failed: ["Échec", "destructive"],
  cancelled: ["Annulé", "outline"],
  cancelling: ["Annulation…", "warning"],
  pending: ["En attente", "warning"],
  approved: ["Approuvé", "success"],
  rejected: ["Refusé", "destructive"],
  executed: ["Exécuté", "success"],
  expired: ["Expiré", "outline"],
  reachable: ["Joignable", "success"],
  unreachable: ["Injoignable", "destructive"],
}

export function statusLabel(status: string): string {
  return STATUS[status]?.[0] ?? status
}

export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const [text, variant] = STATUS[status] ?? [status, "outline" as Variant]
  const dot =
    variant === "success" ? "bg-success" : variant === "destructive" ? "bg-destructive" : variant === "warning" ? "bg-warning" : variant === "info" ? "bg-primary" : "bg-muted-foreground"
  return (
    <Badge variant={variant}>
      <span className={`size-1.5 rounded-full ${dot} ${status === "in_progress" || status === "starting" ? "animate-pulse" : ""}`} />
      {label ?? text}
    </Badge>
  )
}
