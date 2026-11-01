import { useEffect, useRef, useState } from "react"
import { ArrowDownIcon, EraserIcon } from "lucide-react"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useSocket } from "@/lib/socket"
import { resourceApi, type ResourceRef } from "./resource"

const MAX_LINES = 5000
// Codes de couleur ANSI des logs Docker (non interprétés ici).
// eslint-disable-next-line no-control-regex -- on cherche justement le caractère ESC
const ANSI = /\x1b\[[0-9;?]*[A-Za-z]/g
const TAILS = ["100", "500", "1000", "5000"]

/**
 * Onglet « Logs » (Runtime Logs du fork) : `docker logs -f` en direct par WebSocket.
 * `targets` : conteneurs ou sous-services au choix (paramètre `param` de la route de flux).
 */
export function LogsTab({ resource, targets, param }: { resource: ResourceRef; targets?: { value: string; label: string }[]; param?: "app" | "container" }) {
  const [target, setTarget] = useState(targets?.[0]?.value ?? "")
  const [tail, setTail] = useState("100")
  const query = new URLSearchParams({ tail })
  if (param && target) query.set(param, target)
  const path = resourceApi(resource, `/logs/stream?${query}`)
  return (
    <SettingsCard
      title="Logs"
      description="Sortie des conteneurs en direct."
      actions={
        <div className="flex flex-wrap items-center gap-2">
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
          <Select value={tail} onValueChange={setTail}>
            <SelectTrigger className="h-8 w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TAILS.map((t) => (
                <SelectItem key={t} value={t}>
                  {t} dernières lignes
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      }
      bodyClassName="p-0"
    >
      {/* Une nouvelle clé recrée le flux quand la cible ou la longueur change. */}
      <LogStream key={path} path={path} />
    </SettingsCard>
  )
}

/** Affichage d'un flux de logs (messages binaires = texte brut, messages texte = JSON {type…}). */
export function LogStream({ path, height = "h-[60vh]" }: { path: string; height?: string }) {
  const [lines, setLines] = useState<string[]>([])
  const [notice, setNotice] = useState("")
  const [follow, setFollow] = useState(true)
  const box = useRef<HTMLPreElement>(null)
  const partial = useRef("")
  const state = useSocket(path, {
    onBinary: (data) => {
      const text = partial.current + new TextDecoder().decode(data).replace(ANSI, "")
      const parts = text.split("\n")
      partial.current = parts.pop() ?? ""
      setLines((l) => [...l, ...parts].slice(-MAX_LINES))
    },
    onText: (data) => {
      try {
        const msg = JSON.parse(data) as { type: string; message?: string }
        if (msg.type === "error") setNotice(msg.message ?? "Erreur")
        if (msg.type === "end") setNotice("Flux terminé.")
      } catch {
        // message inattendu : ignoré
      }
    },
  })
  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [lines, follow])
  return (
    <div className="relative">
      <div className="flex items-center justify-between gap-2 border-b border-border px-4 py-2 text-xs text-muted-foreground">
        <span>
          {state === "open" ? "Connecté" : state === "reconnecting" ? "Reconnexion…" : state === "closed" ? "Fermé" : "Connexion…"}
          {notice && ` · ${notice}`}
        </span>
        <div className="flex gap-1">
          <Button size="sm" variant="ghost" onClick={() => setFollow(!follow)} className={follow ? "text-primary" : undefined}>
            <ArrowDownIcon /> Suivre
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setLines([])}>
            <EraserIcon /> Effacer
          </Button>
        </div>
      </div>
      <pre
        ref={box}
        onScroll={(e) => {
          const el = e.currentTarget
          setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 40)
        }}
        className={`${height} overflow-auto px-4 py-3 font-mono text-xs leading-5 whitespace-pre-wrap`}
      >
        {lines.length ? lines.join("\n") : <span className="text-muted-foreground">En attente de logs…</span>}
      </pre>
    </div>
  )
}
