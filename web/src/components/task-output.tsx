import { useEffect, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { TaskStatus } from "@/components/task-status"
import { useSocket } from "@/lib/socket"
import type { ServerTask } from "@/lib/types"

/**
 * Sortie d'une tâche serveur en direct (`/servers/{id}/tasks/{task}/stream`) :
 * `{type: "output", data}`, `{type: "reset"}`, `{type: "status", status}`, `{type: "end", status, error}`.
 */
export function TaskOutput({ team, server, task, height = "max-h-96", onEnd }: { team: string; server: string; task: string; height?: string; onEnd?: (status: ServerTask["status"]) => void }) {
  const qc = useQueryClient()
  const [output, setOutput] = useState("")
  const [status, setStatus] = useState<ServerTask["status"]>("queued")
  const [error, setError] = useState("")
  const box = useRef<HTMLPreElement>(null)
  const ended = useRef(onEnd)
  useEffect(() => {
    ended.current = onEnd
  })
  useSocket(`/teams/${team}/servers/${server}/tasks/${task}/stream`, {
    onText: (data) => {
      try {
        const msg = JSON.parse(data) as { type: string; data?: string; status?: ServerTask["status"]; error?: string }
        if (msg.type === "reset") setOutput("")
        if (msg.type === "output" && msg.data) setOutput((o) => o + msg.data)
        if (msg.status) setStatus(msg.status)
        if (msg.type === "end") {
          setError(msg.error ?? "")
          void qc.invalidateQueries({ queryKey: ["team", team, "server", server] })
          if (msg.status) ended.current?.(msg.status)
        }
      } catch {
        // message inattendu : ignoré
      }
    },
  })
  useEffect(() => {
    if (box.current) box.current.scrollTop = box.current.scrollHeight
  }, [output])
  return (
    <div className="space-y-2">
      <TaskStatus status={status} />
      <pre ref={box} className={`${height} min-h-24 overflow-auto rounded-lg border border-border bg-black/40 px-3 py-2 font-mono text-xs leading-5 whitespace-pre-wrap`}>
        {output || <span className="text-muted-foreground">En attente de sortie…</span>}
      </pre>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  )
}
