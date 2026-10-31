import { useEffect, useRef, useState } from "react"
import { wsURL, type RequestOptions } from "./api"

export type SocketState = "connecting" | "open" | "closed" | "reconnecting"

export interface SocketHandlers {
  /** Message texte décodé (JSON). */
  onMessage?: (msg: { type: string; [k: string]: unknown }) => void
  /** Message binaire (octets bruts : logs suivis, terminal). */
  onBinary?: (data: Uint8Array) => void
  onOpen?: (ws: WebSocket) => void
}

/**
 * WebSocket de l'API avec reconnexion (délai croissant) tant que `reconnect` est vrai.
 * Un message `{type: "end" | "exit"}` ou une fermeture normale arrête les reconnexions.
 */
export function useSocket(
  path: string | null,
  query: RequestOptions["query"],
  handlers: SocketHandlers,
  opts: { reconnect?: boolean } = {},
) {
  const [state, setState] = useState<SocketState>("connecting")
  const handlersRef = useRef(handlers)
  const socketRef = useRef<WebSocket | null>(null)
  useEffect(() => {
    handlersRef.current = handlers
  })
  const queryKey = JSON.stringify(query ?? {})
  const reconnect = opts.reconnect ?? true

  useEffect(() => {
    if (!path) return
    let stopped = false
    let finished = false
    let attempt = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    const connect = () => {
      setState(attempt === 0 ? "connecting" : "reconnecting")
      const ws = new WebSocket(wsURL(path, JSON.parse(queryKey)))
      ws.binaryType = "arraybuffer"
      socketRef.current = ws
      ws.onopen = () => {
        attempt = 0
        setState("open")
        handlersRef.current.onOpen?.(ws)
      }
      ws.onmessage = (ev) => {
        if (typeof ev.data === "string") {
          try {
            const msg = JSON.parse(ev.data)
            if (msg.type === "end" || msg.type === "exit") finished = true
            handlersRef.current.onMessage?.(msg)
          } catch {
            // message illisible ignoré
          }
        } else {
          handlersRef.current.onBinary?.(new Uint8Array(ev.data as ArrayBuffer))
        }
      }
      ws.onclose = (ev) => {
        socketRef.current = null
        if (stopped || finished || ev.code === 1000 || !reconnect) {
          setState("closed")
          return
        }
        attempt++
        setState("reconnecting")
        timer = setTimeout(connect, Math.min(30_000, 1000 * 2 ** Math.min(attempt, 5)))
      }
    }
    connect()
    return () => {
      stopped = true
      clearTimeout(timer)
      socketRef.current?.close(1000)
    }
  }, [path, queryKey, reconnect])

  return {
    state,
    send: (data: string | ArrayBufferLike | Uint8Array) => {
      const ws = socketRef.current
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(data as string)
    },
  }
}
