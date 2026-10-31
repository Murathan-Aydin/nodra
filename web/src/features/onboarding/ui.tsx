import { useQuery } from "@tanstack/react-query"
import { useEffect, useRef, type ReactNode } from "react"
import { CheckIcon } from "lucide-react"
import { get, teamPath } from "@/lib/api"
import type { ServerTask } from "@/lib/types"
import { cn } from "@/lib/utils"

const STEPS = ["Serveur", "Connexion", "Terminé"]

/** Barre d'étapes de la configuration guidée (index 0 à 2). */
export function Stepper({ current }: { current: number }) {
  return (
    <ol className="glass-panel grid w-full grid-cols-3 overflow-hidden rounded-2xl text-sm">
      {STEPS.map((label, i) => (
        <li
          key={label}
          className={cn(
            "flex items-center justify-center gap-2.5 border-l border-border px-3 py-3 first:border-l-0",
            i === current ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground",
          )}
          aria-current={i === current ? "step" : undefined}
        >
          <span
            className={cn(
              "flex size-6 items-center justify-center rounded-full border text-xs",
              i < current ? "border-success/40 bg-success/15 text-success" : i === current ? "border-primary/50" : "border-border",
            )}
          >
            {i < current ? <CheckIcon className="size-3.5" /> : i + 1}
          </span>
          {label}
        </li>
      ))}
    </ol>
  )
}

/** Carte de choix (type de serveur, mode de clé…). */
export function ChoiceCard({
  icon,
  title,
  text,
  selected,
  disabled,
  onClick,
  children,
}: {
  icon: ReactNode
  title: string
  text: string
  selected?: boolean
  disabled?: boolean
  onClick: () => void
  children?: ReactNode
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "flex flex-col items-start gap-3 rounded-2xl border border-border bg-muted p-5 text-left transition-all hover:border-primary/50 hover:bg-accent/60 disabled:pointer-events-none disabled:opacity-50",
        selected && "border-primary bg-accent ring-1 ring-primary",
      )}
    >
      <span className="text-muted-foreground [&_svg]:size-7">{icon}</span>
      <span className="space-y-1">
        <span className="block font-medium">{title}</span>
        <span className="block text-sm text-muted-foreground">{text}</span>
      </span>
      {children}
    </button>
  )
}

/** Barre d'actions secondaires en bas d'écran. */
export function FooterActions({ children }: { children: ReactNode }) {
  return <div className="glass-panel mt-8 flex items-center gap-1 rounded-2xl p-1.5">{children}</div>
}

const ACTIVE = new Set(["queued", "running"])

/** Tâche serveur suivie (interrogation régulière) avec sa sortie. */
export function useServerTask(team: string, server?: string, task?: string) {
  return useQuery({
    queryKey: ["team", team, "server", server, "task", task],
    queryFn: () => get<ServerTask>(teamPath(team, `/servers/${server}/tasks/${task}`)),
    enabled: !!server && !!task,
    refetchInterval: (q) => (!q.state.data || ACTIVE.has(q.state.data.status) ? 1000 : false),
  })
}

export function TaskLog({ task }: { task?: ServerTask }) {
  const box = useRef<HTMLPreElement>(null)
  useEffect(() => {
    if (box.current) box.current.scrollTop = box.current.scrollHeight
  }, [task?.output])
  return (
    <pre ref={box} className="max-h-72 min-h-24 overflow-auto rounded-xl border border-border bg-black/70 p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap text-zinc-100">
      {task?.output || (task && ACTIVE.has(task.status) ? "En attente de la sortie…" : "—")}
      {task?.error && <span className="mt-2 block text-red-400">{task.error}</span>}
    </pre>
  )
}
