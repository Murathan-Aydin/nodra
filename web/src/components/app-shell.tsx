import { Link, useNavigate, useRouterState } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useState, type ReactNode } from "react"
import {
  ChevronsUpDownIcon,
  CheckIcon,
  LogOutIcon,
  PaletteIcon,
  PanelLeftIcon,
  SearchIcon,
  UserIcon,
} from "lucide-react"
import { NAV } from "@/components/nav"
import { AppearanceDialog } from "@/components/appearance-dialog"
import { CommandPalette } from "@/components/command-palette"
import { Logo } from "@/components/logo"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { post } from "@/lib/api"
import { instanceQuery, meQuery, teamsQuery } from "@/lib/queries"
import { cn } from "@/lib/utils"

const COLLAPSED_KEY = "nodra-sidebar-collapsed"

/** Libellé de la page courante pour le fil d'Ariane. */
function currentLabel(pathname: string, team: string): string {
  const rest = pathname.slice(`/t/${team}`.length).replace(/\/$/, "")
  for (const g of NAV) for (const i of g.items) if (i.path && (rest === i.path || rest.startsWith(i.path + "/"))) return i.label
  return rest === "" ? "Tableau de bord" : ""
}

export function AppShell({ team, children }: { team: string; children: ReactNode }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(COLLAPSED_KEY) === "1"
    } catch {
      return false
    }
  })
  const [search, setSearch] = useState(false)
  const [appearance, setAppearance] = useState(false)
  useEffect(() => {
    try {
      localStorage.setItem(COLLAPSED_KEY, collapsed ? "1" : "0")
    } catch {
      // stockage local indisponible : le réglage reste en mémoire
    }
  }, [collapsed])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault()
        setSearch(true)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])

  return (
    <div className="flex min-h-screen flex-col gap-3 p-3">
      <TopBar team={team} page={currentLabel(pathname, team)} />
      <div className="flex flex-1 gap-3">
        <aside className={cn("glass-panel sticky top-3 flex h-[calc(100vh-5.25rem)] shrink-0 flex-col rounded-2xl p-3 transition-[width]", collapsed ? "w-16" : "w-64")}>
          <button
            type="button"
            onClick={() => setSearch(true)}
            className={cn(
              "mb-3 flex h-9 items-center gap-2 rounded-lg border border-border bg-muted px-2.5 text-sm text-muted-foreground hover:text-foreground",
              collapsed && "justify-center px-0",
            )}
            aria-label="Rechercher"
          >
            <SearchIcon className="size-4 shrink-0" />
            {!collapsed && (
              <>
                <span className="flex-1 text-left">Rechercher…</span>
                <kbd className="rounded border border-border px-1.5 font-mono text-[10px]">⌘K</kbd>
              </>
            )}
          </button>
          <nav className="flex-1 space-y-5 overflow-y-auto">
            {NAV.map((g, i) => (
              <div key={i} className="space-y-0.5">
                {g.title && !collapsed && <p className="px-2.5 pb-1 text-[11px] font-medium tracking-wider text-muted-foreground uppercase">{g.title}</p>}
                {g.items.map((item) => {
                  const href = `/t/${team}${item.path}`
                  const active = item.path === "" ? pathname === `/t/${team}` || pathname === `/t/${team}/` : pathname.startsWith(href)
                  return (
                    <Link
                      key={item.label}
                      to={href}
                      title={collapsed ? item.label : undefined}
                      className={cn(
                        "flex h-9 items-center gap-3 rounded-lg px-2.5 text-sm transition-colors",
                        active ? "bg-sidebar-accent font-medium text-foreground" : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
                        collapsed && "justify-center px-0",
                      )}
                    >
                      <item.icon className="size-[18px] shrink-0" />
                      {!collapsed && item.label}
                    </Link>
                  )
                })}
              </div>
            ))}
          </nav>
          <div className={cn("mt-3 flex items-center gap-2", collapsed && "flex-col")}>
            <UserMenu team={team} collapsed={collapsed} onAppearance={() => setAppearance(true)} />
            <button
              type="button"
              onClick={() => setCollapsed(!collapsed)}
              className="flex size-9 shrink-0 items-center justify-center rounded-lg text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
              aria-label={collapsed ? "Déplier la barre latérale" : "Replier la barre latérale"}
            >
              <PanelLeftIcon className="size-4" />
            </button>
          </div>
        </aside>
        <main className="min-w-0 flex-1 pb-6">{children}</main>
      </div>
      <CommandPalette team={team} open={search} onOpenChange={setSearch} />
      <AppearanceDialog open={appearance} onOpenChange={setAppearance} />
    </div>
  )
}

function TopBar({ team, page }: { team: string; page: string }) {
  const instance = useQuery(instanceQuery).data
  const teams = useQuery(teamsQuery).data ?? []
  const navigate = useNavigate()
  const current = teams.find((t) => t.id === team)
  return (
    <header className="glass-panel flex h-12 w-fit max-w-full items-center rounded-2xl text-sm">
      <Link to="/t/$team" params={{ team }} className="flex items-center gap-2.5 border-r border-border px-4">
        <Logo className="size-6 rounded-md" />
        <span className="font-semibold">{instance?.name ?? "Nodra"}</span>
        {instance?.version && <span className="font-mono text-xs text-muted-foreground">{instance.version}</span>}
      </Link>
      <div className="flex items-center gap-2 px-3">
        <DropdownMenu>
          <DropdownMenuTrigger className="flex items-center gap-2 rounded-lg px-2 py-1 hover:bg-sidebar-accent">
            <span className="flex size-6 items-center justify-center rounded-md bg-muted text-xs font-semibold uppercase">{current?.name.slice(0, 1) ?? "·"}</span>
            <span className="max-w-48 truncate font-medium">{current?.name ?? "Équipe"}</span>
            <ChevronsUpDownIcon className="size-3.5 text-muted-foreground" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-64">
            <DropdownMenuLabel>Équipes</DropdownMenuLabel>
            {teams.map((t) => (
              <DropdownMenuItem key={t.id} onSelect={() => void navigate({ to: "/t/$team", params: { team: t.id } })}>
                <span className="flex-1 truncate">{t.name}</span>
                {t.id === team && <CheckIcon className="size-4" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        {page && (
          <>
            <span className="text-muted-foreground">/</span>
            <span className="pr-2 font-medium">{page}</span>
          </>
        )}
      </div>
    </header>
  )
}

function UserMenu({ team, collapsed, onAppearance }: { team: string; collapsed: boolean; onAppearance: () => void }) {
  const me = useQuery(meQuery).data?.user
  const navigate = useNavigate()
  const qc = useQueryClient()
  const logout = useMutation({
    mutationFn: () => post("/auth/logout"),
    onSuccess: () => {
      qc.clear()
      void navigate({ to: "/login" })
    },
  })
  const initial = (me?.name || me?.email || "?").slice(0, 1).toUpperCase()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className={cn("flex h-11 min-w-0 flex-1 items-center gap-2.5 rounded-xl border border-border bg-muted px-2 text-left hover:bg-sidebar-accent", collapsed && "size-9 flex-none justify-center px-0")}
      >
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">{initial}</span>
        {!collapsed && (
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{me?.name || me?.email}</span>
            <span className="block truncate text-[11px] text-muted-foreground">{me?.email}</span>
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="w-56">
        <DropdownMenuItem onSelect={() => void navigate({ to: `/t/${team}/account` })}>
          <UserIcon /> Mon compte
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onAppearance}>
          <PaletteIcon /> Apparence
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onSelect={() => logout.mutate()}>
          <LogOutIcon /> Se déconnecter
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
