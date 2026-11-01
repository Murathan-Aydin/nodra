import {
  ActivityIcon,
  BrushCleaningIcon,
  CloudIcon,
  FileCodeIcon,
  FileTextIcon,
  FolderKanbanIcon,
  KeyRoundIcon,
  LayoutGridIcon,
  ListIcon,
  NetworkIcon,
  PackageIcon,
  SettingsIcon,
  ShieldAlertIcon,
  ShieldCheckIcon,
  SquareTerminalIcon,
  TerminalIcon,
  WaypointsIcon,
  HistoryIcon,
  SendIcon,
} from "lucide-react"
import { useQuery } from "@tanstack/react-query"
import type { NavMenu } from "@/components/nav"
import { get } from "@/lib/api"
import type { Destination, Server } from "@/lib/types"

/** Menu latéral d'un serveur, organisé comme server/sidebar.blade.php du fork. */
export function serverMenu(team: string, id: string): NavMenu {
  const at = (path: string) => `/t/${team}/servers/${id}${path}`
  return {
    label: "Serveur",
    groups: [
      {
        items: [
          { label: "Général", icon: SettingsIcon, to: at(""), exact: true },
          { label: "Avancé", icon: LayoutGridIcon, to: at("/advanced") },
          { label: "Clé privée", icon: KeyRoundIcon, to: at("/private-key") },
          { label: "Certificat CA", icon: FileTextIcon, to: at("/ca-certificate") },
          { label: "Cloudflare Tunnel", icon: CloudIcon, to: at("/cloudflare") },
        ],
      },
      {
        title: "Proxy",
        items: [
          { label: "Configuration", icon: NetworkIcon, to: at("/proxy"), exact: true },
          { label: "Configurations dynamiques", icon: FileCodeIcon, to: at("/proxy/dynamic") },
          { label: "Certificats TLS", icon: ShieldCheckIcon, to: at("/proxy/certificates") },
          { label: "Logs du proxy", icon: ListIcon, to: at("/proxy/logs") },
        ],
      },
      {
        items: [
          { label: "Ressources", icon: FolderKanbanIcon, to: at("/resources") },
          { label: "Terminal", icon: SquareTerminalIcon, to: at("/terminal") },
          { label: "Commande", icon: TerminalIcon, to: at("/command") },
          { label: "Destinations", icon: WaypointsIcon, to: at("/destinations") },
          { label: "Nettoyage Docker", icon: BrushCleaningIcon, to: at("/cleanup") },
          { label: "Log drains", icon: SendIcon, to: at("/log-drains") },
          { label: "Métriques", icon: ActivityIcon, to: at("/metrics") },
          { label: "Mises à jour", icon: PackageIcon, to: at("/packages") },
          { label: "Tâches", icon: HistoryIcon, to: at("/tasks") },
        ],
      },
      { items: [{ label: "Zone de danger", icon: ShieldAlertIcon, to: at("/danger"), danger: true }] },
    ],
  }
}

export const serverQuery = (team: string, id: string) => ({
  queryKey: ["team", team, "server", id],
  queryFn: () => get<{ server: Server; destinations: Destination[] }>(`/teams/${team}/servers/${id}`),
})

/** Serveur courant (détail et destinations). */
export function useServer(team: string, id: string) {
  const q = useQuery(serverQuery(team, id))
  return { server: q.data?.server, destinations: q.data?.destinations ?? [], isPending: q.isPending }
}

/** Statut lisible d'un serveur. */
export function serverState(s: Server): { label: string; dot: string } {
  if (!s.validated_at && !s.is_usable) return s.validation_error ? { label: "Validation échouée", dot: "bg-destructive" } : { label: "Non validé", dot: "bg-warning" }
  if (!s.is_reachable) return { label: "Injoignable", dot: "bg-destructive" }
  return s.is_usable ? { label: "Opérationnel", dot: "bg-success" } : { label: "Attention requise", dot: "bg-warning" }
}
