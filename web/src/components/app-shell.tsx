import { Link, useNavigate, useRouterState } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import {
  BellIcon,
  CheckIcon,
  ChevronsUpDownIcon,
  ContainerIcon,
  FolderKanbanIcon,
  GitBranchIcon,
  HardDriveIcon,
  LayoutDashboardIcon,
  LogOutIcon,
  MenuIcon,
  MonitorIcon,
  MoonIcon,
  PlusIcon,
  ScrollTextIcon,
  ServerIcon,
  SettingsIcon,
  ShieldCheckIcon,
  SunIcon,
  UserIcon,
  UsersIcon,
  type LucideIcon,
} from "lucide-react"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { get, post, teamPath } from "@/lib/api"
import { instanceQuery, meQuery, teamsQuery } from "@/lib/queries"
import { useTheme, type Theme } from "@/lib/theme"
import type { ApprovalRequest, Team } from "@/lib/types"
import { cn } from "@/lib/utils"
import { useTeamEvents } from "@/features/realtime/use-team-events"
import { CreateTeamDialog } from "@/features/teams/create-team-dialog"

interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  exact?: boolean
}

const NAV: { title?: string; items: NavItem[]; admin?: boolean }[] = [
  {
    items: [
      { to: "/t/$team", label: "Tableau de bord", icon: LayoutDashboardIcon, exact: true },
      { to: "/t/$team/projects", label: "Projets", icon: FolderKanbanIcon },
      { to: "/t/$team/servers", label: "Serveurs", icon: ServerIcon },
    ],
  },
  {
    title: "Intégrations",
    items: [
      { to: "/t/$team/sources", label: "Sources Git", icon: GitBranchIcon },
      { to: "/t/$team/storages", label: "Stockages S3", icon: HardDriveIcon },
      { to: "/t/$team/registries", label: "Registres Docker", icon: ContainerIcon },
      { to: "/t/$team/notifications", label: "Notifications", icon: BellIcon },
    ],
  },
  {
    title: "Équipe",
    items: [
      { to: "/t/$team/team", label: "Membres et accès", icon: UsersIcon },
      { to: "/t/$team/approvals", label: "Approbations", icon: ShieldCheckIcon },
      { to: "/t/$team/audit", label: "Journal d'audit", icon: ScrollTextIcon },
    ],
  },
  {
    title: "Instance",
    admin: true,
    items: [{ to: "/t/$team/admin", label: "Administration", icon: SettingsIcon }],
  },
]

export function AppShell({ team, children }: { team: string; children: ReactNode }) {
  const [mobileOpen, setMobileOpen] = useState(false)
  useTeamEvents(team)
  return (
    <div className="flex min-h-screen">
      <aside className="sticky top-0 hidden h-screen w-64 shrink-0 border-r bg-sidebar lg:block">
        <Sidebar team={team} />
      </aside>
      {mobileOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-black/40" onClick={() => setMobileOpen(false)} />
          <aside className="absolute inset-y-0 left-0 w-72 border-r bg-sidebar shadow-xl">
            <Sidebar team={team} onNavigate={() => setMobileOpen(false)} />
          </aside>
        </div>
      )}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b bg-background/85 px-4 backdrop-blur lg:px-6">
          <Button variant="ghost" size="icon-sm" className="lg:hidden" onClick={() => setMobileOpen(true)} aria-label="Menu">
            <MenuIcon />
          </Button>
          <div className="flex-1" />
          <ApprovalsBell team={team} />
          <ThemeMenu />
          <UserMenu team={team} />
        </header>
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 lg:px-8">{children}</main>
      </div>
    </div>
  )
}

function Sidebar({ team, onNavigate }: { team: string; onNavigate?: () => void }) {
  const { data: me } = useQuery(meQuery)
  const { data: instance } = useQuery(instanceQuery)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 items-center gap-2.5 px-4">
        <Logo />
        <span className="truncate font-semibold tracking-tight">{instance?.name ?? "Nodra"}</span>
      </div>
      <div className="px-3 pb-2">
        <TeamSwitcher team={team} />
      </div>
      <nav className="flex-1 space-y-5 overflow-y-auto px-3 py-2">
        {NAV.filter((g) => !g.admin || me?.user.is_instance_admin).map((group, i) => (
          <div key={i} className="space-y-1">
            {group.title && <p className="px-2 pb-1 text-xs font-medium text-muted-foreground">{group.title}</p>}
            {group.items.map((item) => {
              const href = item.to.replace("$team", team)
              const active = item.exact ? pathname === href || pathname === href + "/" : pathname.startsWith(href)
              const Icon = item.icon
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  params={{ team }}
                  onClick={onNavigate}
                  className={cn(
                    "flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-sidebar-accent",
                    active ? "bg-sidebar-accent font-medium text-foreground" : "text-muted-foreground",
                  )}
                >
                  <Icon className="size-4" />
                  {item.label}
                </Link>
              )
            })}
          </div>
        ))}
      </nav>
      {instance?.version && <p className="px-5 py-3 text-xs text-muted-foreground">Nodra {instance.version}</p>}
    </div>
  )
}

/** Équipes rangées en arborescence (organisations puis sous-équipes). */
function orderTeams(teams: Team[]): { team: Team; depth: number }[] {
  const byParent = new Map<string | null, Team[]>()
  const ids = new Set(teams.map((t) => t.id))
  for (const t of teams) {
    const parent = t.parent_id && ids.has(t.parent_id) ? t.parent_id : null
    byParent.set(parent, [...(byParent.get(parent) ?? []), t])
  }
  const out: { team: Team; depth: number }[] = []
  const walk = (parent: string | null, depth: number) => {
    for (const t of byParent.get(parent) ?? []) {
      out.push({ team: t, depth })
      walk(t.id, depth + 1)
    }
  }
  walk(null, 0)
  return out
}

function TeamSwitcher({ team }: { team: string }) {
  const { data: teams = [] } = useQuery(teamsQuery)
  const navigate = useNavigate()
  const [creating, setCreating] = useState(false)
  const current = teams.find((t) => t.id === team)
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button className="flex w-full items-center gap-2 rounded-lg border bg-card px-2.5 py-2 text-left text-sm shadow-xs hover:bg-accent/50">
            <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-accent text-xs font-semibold text-accent-foreground">
              {(current?.name ?? "?").slice(0, 2).toUpperCase()}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate font-medium">{current?.name ?? "…"}</span>
              <span className="block text-xs text-muted-foreground">{current?.personal ? "Équipe personnelle" : current?.parent_id ? "Sous-équipe" : "Organisation"}</span>
            </span>
            <ChevronsUpDownIcon className="size-4 text-muted-foreground" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-60" align="start">
          <DropdownMenuLabel>Équipes</DropdownMenuLabel>
          {orderTeams(teams).map(({ team: t, depth }) => (
            <DropdownMenuItem key={t.id} onSelect={() => navigate({ to: "/t/$team", params: { team: t.id } })}>
              <span style={{ paddingLeft: depth * 12 }} className="flex-1 truncate">
                {t.name}
              </span>
              {t.id === team && <CheckIcon className="!text-primary" />}
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => setCreating(true)}>
            <PlusIcon /> Nouvelle équipe
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <CreateTeamDialog open={creating} onOpenChange={setCreating} parentId={current && !current.personal ? current.id : undefined} />
    </>
  )
}

function ApprovalsBell({ team }: { team: string }) {
  const { data = [] } = useQuery({
    queryKey: ["team", team, "approvals", "pending"],
    queryFn: () => get<ApprovalRequest[]>(teamPath(team, "/approvals"), { status: "pending" }),
    refetchInterval: 60_000,
  })
  return (
    <Button variant="ghost" size="icon-sm" asChild aria-label="Approbations en attente" title="Approbations en attente">
      <Link to="/t/$team/approvals" params={{ team }} className="relative">
        <ShieldCheckIcon />
        {data.length > 0 && (
          <span className="absolute -top-0.5 -right-0.5 flex min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] leading-4 font-semibold text-primary-foreground">
            {data.length}
          </span>
        )}
      </Link>
    </Button>
  )
}

function ThemeMenu() {
  const { theme, setTheme, resolved } = useTheme()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Thème">
          {resolved === "dark" ? <MoonIcon /> : <SunIcon />}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as Theme)}>
          <DropdownMenuRadioItem value="light">
            <SunIcon className="size-4" /> Clair
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">
            <MoonIcon className="size-4" /> Sombre
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="system">
            <MonitorIcon className="size-4" /> Système
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function UserMenu({ team }: { team: string }) {
  const { data: me } = useQuery(meQuery)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const initials = (me?.user.name || me?.user.email || "?").slice(0, 1).toUpperCase()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className="flex size-8 items-center justify-center rounded-full bg-primary text-sm font-semibold text-primary-foreground" aria-label="Compte">
          {initials}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuLabel className="font-normal">
          <span className="block truncate text-sm font-medium text-foreground">{me?.user.name || me?.user.email}</span>
          {me?.user.name && <span className="block truncate text-xs">{me.user.email}</span>}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => navigate({ to: "/t/$team/account", params: { team } })}>
          <UserIcon /> Mon compte
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onSelect={async () => {
            await post("/auth/logout").catch(() => undefined)
            qc.clear()
            await navigate({ to: "/login" })
          }}
        >
          <LogOutIcon /> Se déconnecter
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
