import { useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import { BoxIcon, DatabaseIcon, FolderKanbanIcon, LayersIcon, SearchIcon, ServerIcon } from "lucide-react"
import { NAV } from "@/components/nav"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { teamQueries } from "@/lib/queries"
import { cn } from "@/lib/utils"

type Entry = { key: string; group: string; label: string; hint?: string; icon: ReactNode; path: string }

const TYPE_ICON = { application: <BoxIcon />, database: <DatabaseIcon />, service: <LayersIcon /> }

/** Recherche globale (⌘K) : pages, projets, serveurs et ressources de l'équipe. */
export function CommandPalette({ team, open, onOpenChange }: { team: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const navigate = useNavigate()
  const projects = useQuery({ ...teamQueries.projects(team), enabled: open }).data ?? []
  const servers = useQuery({ ...teamQueries.servers(team), enabled: open }).data ?? []
  const resources = useQuery({ ...teamQueries.resources(team), enabled: open }).data ?? []
  const [q, setQ] = useState("")
  const [active, setActive] = useState(0)

  const entries: Entry[] = [
    ...NAV.flatMap((g) => g.items.map((i) => ({ key: "nav" + i.path, group: "Pages", label: i.label, icon: <i.icon />, path: i.path }))),
    ...projects.map((p) => ({ key: p.id, group: "Projets", label: p.name, hint: p.description, icon: <FolderKanbanIcon />, path: `/projects/${p.id}` })),
    ...servers.map((s) => ({ key: s.id, group: "Serveurs", label: s.name, hint: s.ip, icon: <ServerIcon />, path: `/servers/${s.id}` })),
    ...resources.map((r) => ({
      key: r.id,
      group: "Ressources",
      label: r.name,
      hint: `${r.project_name} / ${r.environment_name}`,
      icon: TYPE_ICON[r.type],
      path: `/${r.type}s/${r.id}`,
    })),
  ]
  const needle = q.trim().toLowerCase()
  const list = (needle ? entries.filter((e) => e.label.toLowerCase().includes(needle) || e.hint?.toLowerCase().includes(needle)) : entries).slice(0, 40)
  const go = (e: Entry) => {
    onOpenChange(false)
    setQ("")
    void navigate({ to: `/t/${team}${e.path}` as "/" })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogTitle className="sr-only">Rechercher</DialogTitle>
        <div className="flex items-center gap-2 border-b border-border px-4">
          <SearchIcon className="size-4 text-muted-foreground" />
          <input
            autoFocus
            value={q}
            onChange={(e) => {
              setQ(e.target.value)
              setActive(0)
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault()
                setActive((a) => Math.min(a + 1, list.length - 1))
              }
              if (e.key === "ArrowUp") {
                e.preventDefault()
                setActive((a) => Math.max(a - 1, 0))
              }
              if (e.key === "Enter" && list[active]) go(list[active])
            }}
            placeholder="Pages, projets, serveurs, ressources…"
            className="h-12 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            aria-label="Rechercher"
          />
        </div>
        <ul className="max-h-96 overflow-y-auto p-2">
          {list.map((e, i) => (
            <li key={e.key}>
              {(i === 0 || list[i - 1].group !== e.group) && <p className="px-2 pt-2 pb-1 text-[11px] font-medium tracking-wider text-muted-foreground uppercase">{e.group}</p>}
              <button
                type="button"
                onMouseEnter={() => setActive(i)}
                onClick={() => go(e)}
                className={cn("flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left text-sm [&_svg]:size-4 [&_svg]:text-muted-foreground", i === active && "bg-accent")}
              >
                {e.icon}
                <span className="flex-1 truncate">{e.label}</span>
                {e.hint && <span className="truncate text-xs text-muted-foreground">{e.hint}</span>}
              </button>
            </li>
          ))}
          {!list.length && <li className="px-2 py-6 text-center text-sm text-muted-foreground">Aucun résultat.</li>}
        </ul>
      </DialogContent>
    </Dialog>
  )
}
