import { useEffect, useRef, useState } from "react"
import { DownloadIcon, PauseIcon, PlayIcon, Trash2Icon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { stripAnsi } from "@/lib/ansi"
import type { RequestOptions } from "@/lib/api"
import { useSocket } from "@/lib/ws"
import { cn } from "@/lib/utils"

const MAX = 400_000 // caractères conservés

/** Logs suivis en direct (messages binaires), avec pause, filtre, effacement et téléchargement. */
export function LogStream({ path, query, className, filename = "logs.txt" }: { path: string | null; query?: RequestOptions["query"]; className?: string; filename?: string }) {
  const [text, setText] = useState("")
  const [paused, setPaused] = useState(false)
  const [filter, setFilter] = useState("")
  const [end, setEnd] = useState<string>()
  const buffer = useRef("")
  const decoder = useRef(new TextDecoder())
  const pre = useRef<HTMLPreElement>(null)
  const pausedRef = useRef(paused)
  useEffect(() => {
    pausedRef.current = paused
  }, [paused])

  const { state } = useSocket(
    path,
    query,
    {
      onOpen: () => {
        buffer.current = ""
        setText("")
        setEnd(undefined)
      },
      onBinary: (bytes) => {
        buffer.current = (buffer.current + stripAnsi(decoder.current.decode(bytes, { stream: true }))).slice(-MAX)
        if (!pausedRef.current) setText(buffer.current)
      },
      onMessage: (m) => {
        if (m.type === "error") setEnd(String(m.message))
        if (m.type === "end") setEnd("Flux terminé.")
      },
    },
    { reconnect: false },
  )

  useEffect(() => {
    if (!paused && pre.current) pre.current.scrollTop = pre.current.scrollHeight
  }, [text, paused])

  const shown = filter ? text.split("\n").filter((l) => l.toLowerCase().includes(filter.toLowerCase())).join("\n") : text
  return (
    <div className={cn("space-y-2", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={state === "open" ? "success" : state === "closed" ? "secondary" : "info"}>
          {state === "open" ? "En direct" : state === "closed" ? "Arrêté" : "Connexion…"}
        </Badge>
        <Input className="h-8 max-w-xs" placeholder="Filtrer" value={filter} onChange={(e) => setFilter(e.target.value)} />
        <div className="flex-1" />
        <Button size="sm" variant="ghost" onClick={() => setPaused((p) => !p)}>
          {paused ? <PlayIcon /> : <PauseIcon />} {paused ? "Reprendre" : "Pause"}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            buffer.current = ""
            setText("")
          }}
        >
          <Trash2Icon /> Effacer
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            const url = URL.createObjectURL(new Blob([buffer.current], { type: "text/plain" }))
            const a = Object.assign(document.createElement("a"), { href: url, download: filename })
            a.click()
            URL.revokeObjectURL(url)
          }}
        >
          <DownloadIcon /> Télécharger
        </Button>
      </div>
      <pre ref={pre} className="h-[60vh] overflow-auto rounded-lg bg-zinc-950 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-zinc-100">
        {shown || <span className="text-zinc-500">{state === "open" ? "En attente de logs…" : ""}</span>}
      </pre>
      {end && <p className="text-xs text-muted-foreground">{end}</p>}
    </div>
  )
}
