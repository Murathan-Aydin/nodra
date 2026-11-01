import { useEffect, useRef, useState } from "react"
import { useQueryClient, type QueryKey } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

export type SocketState = "connecting" | "open" | "reconnecting" | "closed"

type Handlers = {
  onText?: (data: string) => void
  onBinary?: (data: ArrayBuffer) => void
}

const MAX_DELAY = 30_000

// WebSocket sur la même origine (/api/v1<path>), reconnectée après 1 s, 2 s, 4 s… (30 s max.).
// Une fin propre (code 1000) ne déclenche pas de reconnexion.
export function useSocket(path: string, handlers: Handlers): SocketState {
  const [state, setState] = useState<SocketState>("connecting")
  const handlersRef = useRef(handlers)
  useEffect(() => {
    handlersRef.current = handlers
  })

  useEffect(() => {
    let ws: WebSocket | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let delay = 1000
    let disposed = false

    const connect = () => {
      const proto = location.protocol === "https:" ? "wss:" : "ws:"
      ws = new WebSocket(`${proto}//${location.host}/api/v1${path}`)
      ws.binaryType = "arraybuffer"
      ws.onopen = () => {
        delay = 1000
        setState("open")
      }
      ws.onmessage = (e: MessageEvent<string | ArrayBuffer>) => {
        if (typeof e.data === "string") handlersRef.current.onText?.(e.data)
        else handlersRef.current.onBinary?.(e.data)
      }
      ws.onclose = (e) => {
        ws = null
        if (disposed) return
        if (e.code === 1000) {
          setState("closed")
          return
        }
        setState("reconnecting")
        timer = setTimeout(connect, delay)
        delay = Math.min(delay * 2, MAX_DELAY)
      }
    }

    connect()
    return () => {
      disposed = true
      clearTimeout(timer)
      ws?.close(1000)
      setState("connecting")
    }
  }, [path])

  return state
}

type TeamEvent = { kind: string; id?: string; resource?: string; server?: string; status?: string }

function keysFor(team: string, ev: TeamEvent): QueryKey[] {
  const k = (...rest: unknown[]): QueryKey => ["team", team, ...rest]
  switch (ev.kind) {
    case "resource.status":
      return [k("resources"), k("application", ev.id), k("database", ev.id), k("service", ev.id)]
    case "deployment.status":
      return [k("deployments"), k("deployment", ev.id), k("resources"), k("application", ev.resource)]
    case "deployment.log":
      return [k("deployment", ev.id, "logs")]
    case "task.output":
      return [k("server", ev.server, "task", ev.id)]
    case "task.status":
      return [k("server", ev.server), k("servers")]
    case "server.status":
    case "proxy.status":
      return [k("servers"), k("server", ev.server ?? ev.id)]
    default:
      return []
  }
}

// Écoute les événements de l'équipe et invalide les requêtes TanStack concernées.
export function useTeamEvents(team: string): SocketState {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  return useSocket(`/teams/${team}/events`, {
    onText: (data) => {
      let msg: { type?: string; event?: TeamEvent }
      try {
        msg = JSON.parse(data)
      } catch {
        return
      }
      if (msg.type !== "event" || !msg.event) return
      for (const queryKey of keysFor(team, msg.event)) void queryClient.invalidateQueries({ queryKey })
      const ev = msg.event
      // Fin d'un déploiement : message discret avec un lien vers ses logs.
      if (ev.kind === "deployment.status" && (ev.status === "finished" || ev.status === "failed") && ev.resource) {
        const url = `/t/${team}/applications/${ev.resource}/deployments/${ev.id}`
        const action = { label: "Voir", onClick: () => void navigate({ to: url }) }
        if (ev.status === "finished") toast.success("Déploiement terminé", { action })
        else toast.error("Déploiement échoué", { action })
      }
    },
  })
}
