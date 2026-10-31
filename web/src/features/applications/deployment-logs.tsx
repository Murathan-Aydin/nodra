import { useEffect, useRef, useState } from "react"
import { StatusBadge } from "@/components/status-badge"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import type { DeploymentLog } from "@/lib/types"
import { useSocket } from "@/lib/ws"
import { cn } from "@/lib/utils"

const KIND: Record<string, string> = {
  command: "text-sky-300",
  stderr: "text-amber-300",
  error: "text-red-400 font-medium",
  info: "text-zinc-400",
}

/** Logs d'un déploiement en direct (lignes masquées avec « logs détaillés »). */
export function DeploymentLogs({ base, deployment, onStatus }: { base: string; deployment: string; onStatus?: (s: string) => void }) {
  const [debug, setDebug] = useState(false)
  const [lines, setLines] = useState<DeploymentLog[]>([])
  const [status, setStatus] = useState<string>()
  const [error, setError] = useState("")
  const box = useRef<HTMLDivElement>(null)
  const stick = useRef(true)
  useSocket(
    `${base}/deployments/${deployment}/logs/stream`,
    { debug: debug ? 1 : undefined },
    {
      onOpen: () => setLines([]),
      onMessage: (m) => {
        if (m.type === "logs") setLines((l) => [...l, ...(m.logs as DeploymentLog[])])
        if (m.type === "status" || m.type === "end") {
          setStatus(String(m.status))
          onStatus?.(String(m.status))
        }
        if (m.type === "end") setError(String(m.error ?? ""))
      },
    },
  )
  useEffect(() => {
    if (box.current && stick.current) box.current.scrollTop = box.current.scrollHeight
  }, [lines])
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        {status ? <StatusBadge status={status} /> : <span />}
        <Label className="font-normal">
          <Switch checked={debug} onCheckedChange={setDebug} /> Logs détaillés
        </Label>
      </div>
      <div
        ref={box}
        onScroll={(e) => {
          const el = e.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
        }}
        className="h-[60vh] overflow-auto rounded-lg bg-zinc-950 p-3 font-mono text-xs leading-relaxed text-zinc-100"
      >
        {lines.length === 0 && <p className="text-zinc-500">En attente des logs…</p>}
        {lines.map((l) => (
          <div key={l.id} className={cn("whitespace-pre-wrap", KIND[l.kind], l.hidden && "opacity-70")}>
            <span className="mr-2 text-zinc-600 select-none">{new Date(l.created_at).toLocaleTimeString("fr-FR")}</span>
            {l.message}
          </div>
        ))}
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  )
}
