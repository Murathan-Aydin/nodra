import { useMutation } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import { toast } from "sonner"
import { TaskOutput } from "@/components/task-output"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import type { ServerTask } from "@/lib/types"

/** Lance une tâche serveur (POST → ServerTask) puis affiche sa sortie en direct. */
export function useServerTask(team: string, server: string) {
  const [task, setTask] = useState<string | null>(null)
  const run = useMutation({
    mutationFn: ({ path, method = "POST", json }: { path: string; method?: string; json?: unknown }) => api<ServerTask | { task?: ServerTask; approval_request?: unknown }>(`/teams/${team}/servers/${server}${path}`, { method, json: json ?? {} }),
    onSuccess: (r) => {
      if (r && "approval_request" in r && r.approval_request) return void toast.info("En attente d'approbation")
      const t = r && "task" in r && r.task ? r.task : (r as ServerTask | undefined)
      if (t?.id) setTask(t.id)
    },
    onError: (e) => toast.error(e.message),
  })
  const output = task ? <TaskOutput key={task} team={team} server={server} task={task} /> : null
  return { run, output, reset: () => setTask(null) }
}

/** Bouton qui lance une tâche serveur. */
export function TaskButton({ label, icon, pending, onClick, variant = "outline" }: { label: string; icon?: ReactNode; pending: boolean; onClick: () => void; variant?: "outline" | "default" | "destructive" }) {
  return (
    <Button size="sm" variant={variant} loading={pending} onClick={onClick}>
      {icon} {label}
    </Button>
  )
}
