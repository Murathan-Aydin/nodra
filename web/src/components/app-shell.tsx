import { Link, useNavigate, useRouterState } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { createContext, useContext, useEffect, useState, type ReactNode } from "react"
import { createPortal } from "react-dom"
import {
  ChevronDownIcon,
  MenuIcon,
  LogOutIcon,
  PaletteIcon,
  PanelLeftIcon,
  SearchIcon,
  UserIcon,
} from "lucide-react"
import { SidebarNav } from "@/components/sidebar-nav"
import { TopBreadcrumb } from "@/components/top-breadcrumb"
import { AppearanceDialog } from "@/components/appearance-dialog"
import { CommandPalette } from "@/components/command-palette"
import { Logo } from "@/components/logo"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { post } from "@/lib/api"
import { instanceQuery, meQuery } from "@/lib/queries"
import { cn } from "@/lib/utils"

const COLLAPSED_KEY = "nodra-sidebar-collapsed"

export function AppShell({ team, children }: { team: string; children: ReactNode }) {
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(COLLAPSED_KEY) === "1"
    } catch {
      return false
    }
  })
  const [search, setSearch] = useState(false)
  const [actionsSlot, setActionsSlot] = useState<HTMLElement | null>(null)
  // Tiroir mobile : ouvert seulement sur la page où on l'a ouvert (il se ferme en changeant de page).
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const [drawerOn, setDrawerOn] = useState<string | null>(null)
  const drawer = drawerOn === pathname
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

  // Largeur du panneau d'actions, pour que la barre du haut s'arrête avant lui (--resource-actions-w du fork).
  useEffect(() => {
    if (!actionsSlot) return
    const ro = new ResizeObserver(([entry]) => document.documentElement.style.setProperty("--actions-w", `${entry.borderBoxSize[0].inlineSize}px`))
    ro.observe(actionsSlot)
    return () => ro.disconnect()
  }, [actionsSlot])

  return (
    <div className="app-shell" data-collapsed={collapsed ? "" : undefined} data-mobile-open={drawer ? "" : undefined}>
      <TopBar team={team} collapsed={collapsed} onMenu={() => setDrawerOn(drawer ? null : pathname)} />
      {drawer && <button type="button" aria-label="Fermer le menu" className="fixed inset-0 z-50 bg-black/40 lg:hidden" onClick={() => setDrawerOn(null)} />}

      {/* Actions de la page (resource-action-hud-slot du fork), remplies par <TopBarActions>. */}
      <div ref={setActionsSlot} className="app-actions glass-panel" />

      <aside className="app-sidebar glass-panel">
        <button type="button" onClick={() => setSearch(true)} className="menu-item mb-2 justify-between rounded-lg! bg-muted" aria-label="Rechercher">
          <span className="flex min-w-0 items-center gap-2.5">
            <SearchIcon className="menu-item-icon" />
            {!collapsed && <span className="menu-item-label">Rechercher</span>}
          </span>
          {!collapsed && <kbd className="rounded border border-border px-1.5 font-mono text-[10px] text-muted-foreground">⌘K</kbd>}
        </button>
        <nav className="-mx-1 flex min-h-0 flex-1 flex-col gap-y-0.5 overflow-y-auto px-1 pb-2">
          <SidebarNav team={team} collapsed={collapsed} />
        </nav>
        <div className={cn("flex shrink-0 items-center gap-1 border-t border-border py-2", collapsed && "flex-col")}>
          <UserMenu team={team} collapsed={collapsed} onAppearance={() => setAppearance(true)} />
          <button
            type="button"
            onClick={() => setCollapsed(!collapsed)}
            className="menu-item w-8 shrink-0 justify-center px-0"
            aria-label={collapsed ? "Déplier la barre latérale" : "Replier la barre latérale"}
          >
            <PanelLeftIcon className="menu-item-icon" />
          </button>
        </div>
      </aside>

      <main className="app-main">
        <div className="mx-auto w-full max-w-[1400px]">
          <ActionsSlot.Provider value={actionsSlot}>{children}</ActionsSlot.Provider>
        </div>
      </main>
      <CommandPalette team={team} open={search} onOpenChange={setSearch} />
      <AppearanceDialog open={appearance} onOpenChange={setAppearance} />
    </div>
  )
}

function TopBar({ team, collapsed, onMenu }: { team: string; collapsed: boolean; onMenu: () => void }) {
  const instance = useQuery(instanceQuery).data
  return (
    <header className="app-topbar glass-panel">
      <div className="app-topbar-brand">
        <Link to="/t/$team" params={{ team }} title={instance?.name ?? "Nodra"} className="flex min-w-0 items-baseline gap-1.5 transition-opacity hover:opacity-80">
          {collapsed ? (
            <Logo className="size-5 rounded-md" />
          ) : (
            <>
              <span className="text-[15px] font-semibold tracking-tight">{instance?.name ?? "Nodra"}</span>
              {instance?.version && <span className="truncate text-[10.5px] font-medium text-muted-foreground">{instance.version}</span>}
            </>
          )}
        </Link>
      </div>
      <div className="flex h-full min-w-0 flex-1 items-center gap-0.5 overflow-x-auto pr-4 pl-3 [scrollbar-width:none]">
        <TopBreadcrumb team={team} />
      </div>
      <button type="button" onClick={onMenu} aria-label="Menu" className="mr-2 flex size-9 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-foreground/5 hover:text-foreground lg:hidden">
        <MenuIcon className="size-5" />
      </button>
    </header>
  )
}

const ActionsSlot = createContext<HTMLElement | null>(null)

/** Place ses enfants dans la barre du haut, à droite (boutons d'action de la page). */
export function TopBarActions({ children }: { children: ReactNode }) {
  const slot = useContext(ActionsSlot)
  return slot ? createPortal(children, slot) : null
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
        className={cn("menu-item min-w-0 flex-1 rounded-lg! border border-border", collapsed && "w-8 flex-none justify-center px-0")}
      >
        <span className="flex size-5 shrink-0 items-center justify-center rounded-md bg-muted text-[11px] font-semibold text-foreground">{initial}</span>
        {!collapsed && (
          <>
            <span className="menu-item-label">{me?.name || me?.email}</span>
            <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
          </>
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
