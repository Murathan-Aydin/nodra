import type { ServerTask } from "@/lib/types"
import { cn } from "@/lib/utils"

const TASK: Record<ServerTask["status"], { label: string; dot: string }> = {
  queued: { label: "En attente", dot: "bg-muted-foreground" },
  running: { label: "En cours", dot: "bg-primary animate-pulse" },
  success: { label: "Réussie", dot: "bg-success" },
  failed: { label: "Échouée", dot: "bg-destructive" },
  cancelled: { label: "Annulée", dot: "bg-warning" },
}

/** Pastille de statut d'une tâche serveur (même forme que la pastille des ressources). */
export function TaskStatus({ status }: { status: ServerTask["status"] }) {
  const s = TASK[status] ?? TASK.queued
  return (
    <span className="inline-flex h-[22px] shrink-0 items-center gap-1.5 rounded-full border border-border bg-muted px-2.5 text-xs font-medium">
      <span className={cn("size-1.5 rounded-full", s.dot)} />
      {s.label}
    </span>
  )
}

/** Pastille générique : libellé + couleur de point. */
export function Pill({ label, dot }: { label: string; dot: string }) {
  return (
    <span className="inline-flex h-[22px] shrink-0 items-center gap-1.5 rounded-full border border-border bg-muted px-2.5 text-xs font-medium">
      <span className={cn("size-1.5 rounded-full", dot)} />
      {label}
    </span>
  )
}
