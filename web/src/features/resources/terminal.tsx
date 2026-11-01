import { useEffect, useRef, useState } from "react"
import { Terminal } from "@xterm/xterm"
import { FitAddon } from "@xterm/addon-fit"
import "@xterm/xterm/css/xterm.css"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

type Status = "idle" | "connecting" | "ready" | "closed"

/**
 * Terminal web (xterm) relié à une route WebSocket `…/terminal` :
 * saisie en binaire, `{type: "resize", rows, cols}`, sortie en binaire, `{type: "exit", code}`.
 */
export function WebTerminal({ path, onStatus }: { path: string; onStatus?: (s: Status, info?: string) => void }) {
  const el = useRef<HTMLDivElement>(null)
  const status = useRef(onStatus)
  useEffect(() => {
    status.current = onStatus
  })
  useEffect(() => {
    if (!el.current) return
    const term = new Terminal({ fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace", fontSize: 13, cursorBlink: true, theme: { background: "#00000000" }, allowTransparency: true })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(el.current)
    fit.fit()
    const proto = location.protocol === "https:" ? "wss:" : "ws:"
    const sep = path.includes("?") ? "&" : "?"
    const ws = new WebSocket(`${proto}//${location.host}/api/v1${path}${sep}rows=${term.rows}&cols=${term.cols}`)
    ws.binaryType = "arraybuffer"
    const enc = new TextEncoder()
    status.current?.("connecting")
    ws.onmessage = (e: MessageEvent<string | ArrayBuffer>) => {
      if (typeof e.data !== "string") return term.write(new Uint8Array(e.data))
      try {
        const msg = JSON.parse(e.data) as { type: string; code?: number; container?: string; message?: string }
        if (msg.type === "ready") status.current?.("ready", msg.container)
        if (msg.type === "exit") {
          term.write(`\r\n\x1b[2m[session terminée, code ${msg.code ?? 0}]\x1b[0m\r\n`)
          status.current?.("closed")
        }
        if (msg.type === "error") term.write(`\r\n\x1b[31m${msg.message}\x1b[0m\r\n`)
      } catch {
        // message inattendu : ignoré
      }
    }
    ws.onclose = (e) => {
      if (e.code !== 1000 && e.reason) term.write(`\r\n\x1b[31m${e.reason}\x1b[0m\r\n`)
      status.current?.("closed")
    }
    const input = term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)))
    const resize = term.onResize(({ rows, cols }) => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ type: "resize", rows, cols })))
    const ro = new ResizeObserver(() => fit.fit())
    ro.observe(el.current)
    term.focus()
    return () => {
      ro.disconnect()
      input.dispose()
      resize.dispose()
      ws.close(1000)
      term.dispose()
    }
  }, [path])
  return <div ref={el} className="h-[60vh] w-full overflow-hidden px-3 py-2" />
}

/** Onglet « Terminal » d'une ressource : choix du conteneur puis connexion à la demande. */
export function TerminalTab({ path, targets, param }: { path: string; targets?: { value: string; label: string }[]; param?: string }) {
  const [target, setTarget] = useState(targets?.[0]?.value ?? "")
  const [session, setSession] = useState(0)
  const [state, setState] = useState<{ s: Status; info?: string }>({ s: "idle" })
  const full = param && target ? `${path}${path.includes("?") ? "&" : "?"}${param}=${encodeURIComponent(target)}` : path
  return (
    <SettingsCard
      title="Terminal"
      description={state.s === "ready" ? `Connecté${state.info ? ` à ${state.info}` : ""}. L'ouverture est journalisée.` : "Shell dans le conteneur en cours d'exécution. L'ouverture est journalisée."}
      actions={
        <div className="flex items-center gap-2">
          {targets && targets.length > 1 && (
            <Select value={target} onValueChange={setTarget}>
              <SelectTrigger className="h-8 w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {targets.map((t) => (
                  <SelectItem key={t.value} value={t.value}>
                    {t.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <Button size="sm" onClick={() => setSession((n) => n + 1)}>
            {session ? "Reconnecter" : "Se connecter"}
          </Button>
        </div>
      }
      bodyClassName="p-0 bg-black/60"
    >
      {session ? (
        <WebTerminal key={`${full}#${session}`} path={full} onStatus={(s, info) => setState({ s, info })} />
      ) : (
        <p className="px-5 py-10 text-center text-sm text-muted-foreground">Cliquez sur « Se connecter » pour ouvrir un shell.</p>
      )}
    </SettingsCard>
  )
}
