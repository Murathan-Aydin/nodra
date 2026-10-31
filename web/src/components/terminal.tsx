import { useEffect, useRef, useState } from "react"
import { Terminal as XTerm } from "@xterm/xterm"
import { FitAddon } from "@xterm/addon-fit"
import "@xterm/xterm/css/xterm.css"
import { MaximizeIcon, MinimizeIcon, RotateCcwIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { wsURL, type RequestOptions } from "@/lib/api"
import { cn } from "@/lib/utils"

type State = "connecting" | "ready" | "closed" | "error"

/**
 * Terminal web relié à une route WebSocket de l'API (serveur ou conteneur) : saisie et sortie en
 * binaire, redimensionnement par message {type: "resize"}.
 */
export function WebTerminal({ path, query, className }: { path: string; query?: RequestOptions["query"]; className?: string }) {
  const host = useRef<HTMLDivElement>(null)
  const [state, setState] = useState<State>("connecting")
  const [info, setInfo] = useState("")
  const [session, setSession] = useState(0)
  const [full, setFull] = useState(false)
  const queryKey = JSON.stringify(query ?? {})

  useEffect(() => {
    if (!host.current) return
    const term = new XTerm({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, Menlo, monospace',
      fontSize: 13,
      theme: { background: "#09090b", foreground: "#e4e4e7", cursor: "#a5b4fc" },
      scrollback: 5000,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host.current)
    fit.fit()
    setState("connecting")
    setInfo("")
    const ws = new WebSocket(wsURL(path, { ...JSON.parse(queryKey), rows: term.rows, cols: term.cols }))
    ws.binaryType = "arraybuffer"
    const enc = new TextEncoder()
    ws.onmessage = (ev) => {
      if (typeof ev.data !== "string") {
        term.write(new Uint8Array(ev.data as ArrayBuffer))
        return
      }
      try {
        const m = JSON.parse(ev.data)
        if (m.type === "ready") {
          setState("ready")
          if (m.container) setInfo(m.container)
          term.focus()
        } else if (m.type === "exit") {
          setState("closed")
          term.write(`\r\n\x1b[90m[session terminée, code ${m.code}]\x1b[0m\r\n`)
        } else if (m.type === "error") {
          setState("error")
          term.write(`\r\n\x1b[31m${m.message}\x1b[0m\r\n`)
        }
      } catch {
        // message illisible ignoré
      }
    }
    ws.onclose = () => setState((s) => (s === "ready" || s === "connecting" ? "closed" : s))
    const input = term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)))
    const resize = term.onResize(({ rows, cols }) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: "resize", rows, cols }))
    })
    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
      } catch {
        // terminal démonté
      }
    })
    ro.observe(host.current)
    return () => {
      ro.disconnect()
      input.dispose()
      resize.dispose()
      ws.close(1000)
      term.dispose()
    }
  }, [path, queryKey, session])

  return (
    <div className={cn("flex flex-col overflow-hidden rounded-lg border bg-zinc-950", full && "fixed inset-4 z-50 shadow-2xl", className)}>
      <div className="flex items-center gap-2 border-b border-white/10 px-3 py-1.5 text-xs text-zinc-400">
        <Badge variant={state === "ready" ? "success" : state === "connecting" ? "info" : state === "error" ? "destructive" : "secondary"}>
          {state === "ready" ? "Connecté" : state === "connecting" ? "Connexion…" : state === "error" ? "Erreur" : "Fermé"}
        </Badge>
        {info && <span className="font-mono">{info}</span>}
        <div className="flex-1" />
        {(state === "closed" || state === "error") && (
          <Button size="sm" variant="ghost" className="h-7 text-zinc-300 hover:bg-white/10 hover:text-white" onClick={() => setSession((s) => s + 1)}>
            <RotateCcwIcon /> Reconnecter
          </Button>
        )}
        <Button size="icon-sm" variant="ghost" className="size-7 text-zinc-300 hover:bg-white/10 hover:text-white" onClick={() => setFull((f) => !f)} aria-label="Plein écran">
          {full ? <MinimizeIcon /> : <MaximizeIcon />}
        </Button>
      </div>
      <div ref={host} className={cn("min-h-0 flex-1 p-2", full ? "" : "h-[60vh]")} />
    </div>
  )
}
