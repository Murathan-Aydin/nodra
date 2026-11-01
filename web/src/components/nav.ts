import {
  ActivityIcon,
  BellIcon,
  BoxesIcon,
  ClipboardCheckIcon,
  CloudIcon,
  CodeIcon,
  FolderKanbanIcon,
  GitBranchIcon,
  HistoryIcon,
  HomeIcon,
  KeyRoundIcon,
  LayoutGridIcon,
  MailIcon,
  PlugIcon,
  RefreshCwIcon,
  SendIcon,
  ServerIcon,
  SettingsIcon,
  ShieldAlertIcon,
  ShieldIcon,
  SquareTerminalIcon,
  TagIcon,
  UserCogIcon,
  UsersIcon,
  UsersRoundIcon,
  VariableIcon,
  type LucideIcon,
} from "lucide-react"

/** Entrée de menu. `path` est relatif à /t/$team ; `children` ouvre un sous-menu à la place du menu principal. */
export type NavItem = { label: string; icon: LucideIcon; path: string; children?: NavItem[]; sectionStart?: boolean; danger?: boolean; admin?: boolean }

/** Navigation principale (même organisation que la barre latérale du fork Coolify). */
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
      {
        label: "Intégrations",
        icon: PlugIcon,
        path: "/integrations",
        children: [
          { label: "Sources", icon: GitBranchIcon, path: "/integrations/sources" },
          { label: "Registres", icon: BoxesIcon, path: "/integrations/registries" },
          { label: "Stockages S3", icon: CloudIcon, path: "/integrations/s3" },
        ],
      },
      { label: "Variables partagées", icon: VariableIcon, path: "/variables" },
    ],
  },
  {
    title: "Gestion",
    items: [
      {
        label: "Équipe",
        icon: UsersIcon,
        path: "/team",
        children: [
          { label: "Général", icon: SettingsIcon, path: "/team" },
          { label: "Membres", icon: UsersIcon, path: "/team/members" },
          { label: "Rôles", icon: ShieldIcon, path: "/team/roles" },
          { label: "Groupes", icon: UsersRoundIcon, path: "/team/groups" },
          { label: "Approbations", icon: ClipboardCheckIcon, path: "/team/approvals" },
          { label: "Journal d'audit", icon: HistoryIcon, path: "/team/audit" },
          { label: "Zone de danger", icon: ShieldAlertIcon, path: "/team/danger", sectionStart: true, danger: true },
        ],
      },
      {
        label: "Notifications",
        icon: BellIcon,
        path: "/notifications",
        children: [
          { label: "Canaux", icon: SendIcon, path: "/notifications" },
          { label: "Abonnements", icon: BellIcon, path: "/notifications/events" },
          { label: "Journal des envois", icon: HistoryIcon, path: "/notifications/deliveries" },
        ],
      },
      {
        label: "Clés et jetons",
        icon: KeyRoundIcon,
        path: "/keys",
        children: [
          { label: "Clés SSH", icon: KeyRoundIcon, path: "/keys" },
          { label: "Jetons API", icon: CodeIcon, path: "/keys/tokens" },
        ],
      },
      { label: "Tags", icon: TagIcon, path: "/tags" },
      {
        label: "Réglages",
        icon: SettingsIcon,
        path: "/settings",
        admin: true,
        children: [
          { label: "Général", icon: SettingsIcon, path: "/settings" },
          { label: "Avancé", icon: LayoutGridIcon, path: "/settings/advanced" },
          { label: "Mises à jour", icon: RefreshCwIcon, path: "/settings/updates" },
          { label: "Authentification", icon: KeyRoundIcon, path: "/settings/oauth", sectionStart: true },
          { label: "E-mail", icon: MailIcon, path: "/settings/email" },
          { label: "Utilisateurs", icon: UserCogIcon, path: "/settings/users" },
          { label: "Audit de l'instance", icon: HistoryIcon, path: "/settings/audit" },
        ],
      },
    ],
  },
]

/** Menu propre à une page (ressource, serveur…) : remplace le menu principal, avec un retour. */
export type NavMenu = {
  label: string
  groups: { title?: string; items: { label: string; icon: LucideIcon; to: string; exact?: boolean; danger?: boolean }[] }[]
}

// Les routes déclarent leur menu dans staticData.nav (lu par la barre latérale).
declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    nav?: (params: Record<string, string>) => NavMenu
  }
}
