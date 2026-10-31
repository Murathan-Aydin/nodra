import {
  ActivityIcon,
  BellIcon,
  FolderKanbanIcon,
  HomeIcon,
  KeyRoundIcon,
  PlugIcon,
  ServerIcon,
  SettingsIcon,
  SquareTerminalIcon,
  TagIcon,
  UsersIcon,
  VariableIcon,
  type LucideIcon,
} from "lucide-react"

type NavItem = { label: string; icon: LucideIcon; path: string }

/** Navigation principale (même organisation que Coolify). `path` est relatif à /t/$team. */
export const NAV: { title?: string; items: NavItem[] }[] = [
  {
    items: [
      { label: "Tableau de bord", icon: HomeIcon, path: "" },
      { label: "Projets", icon: FolderKanbanIcon, path: "/projects" },
      { label: "Supervision", icon: ActivityIcon, path: "/monitoring" },
      { label: "Terminal", icon: SquareTerminalIcon, path: "/terminal" },
    ],
  },
  {
    title: "Infrastructure",
    items: [
      { label: "Serveurs", icon: ServerIcon, path: "/servers" },
      { label: "Intégrations", icon: PlugIcon, path: "/integrations" },
      { label: "Variables partagées", icon: VariableIcon, path: "/variables" },
    ],
  },
  {
    title: "Gestion",
    items: [
      { label: "Équipe", icon: UsersIcon, path: "/team" },
      { label: "Notifications", icon: BellIcon, path: "/notifications" },
      { label: "Clés et jetons", icon: KeyRoundIcon, path: "/keys" },
      { label: "Tags", icon: TagIcon, path: "/tags" },
      { label: "Réglages", icon: SettingsIcon, path: "/settings" },
    ],
  },
]
