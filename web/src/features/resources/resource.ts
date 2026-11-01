import {
  ActivityIcon,
  BoxIcon,
  CalendarClockIcon,
  CodeXmlIcon,
  DatabaseBackupIcon,
  GitBranchIcon,
  GitPullRequestIcon,
  GlobeIcon,
  HardDriveIcon,
  HistoryIcon,
  ListIcon,
  RocketIcon,
  ServerCogIcon,
  SettingsIcon,
  ShieldAlertIcon,
  SlidersHorizontalIcon,
  SquareTerminalIcon,
  TagIcon,
  UploadIcon,
  VariableIcon,
  WebhookIcon,
} from "lucide-react"
import type { NavMenu } from "@/components/nav"

export type ResourceKind = "application" | "database" | "service"

/** Ressource visée par un onglet : équipe, type et identifiant. */
export type ResourceRef = { team: string; kind: ResourceKind; id: string }

const PLURAL: Record<ResourceKind, string> = { application: "applications", database: "databases", service: "services" }

/** Chemin API de la ressource (`/teams/{team}/services/{id}`…). */
export const resourceApi = (r: ResourceRef, path = "") => `/teams/${r.team}/${PLURAL[r.kind]}/${r.id}${path}`

/** Clé TanStack de la ressource (invalidée par useTeamEvents). */
export const resourceKey = (r: ResourceRef, ...rest: unknown[]) => ["team", r.team, r.kind, r.id, ...rest]

/** Adresse de la page d'une ressource dans le front. */
export const resourcePage = (r: ResourceRef, path = "") => `/t/${r.team}/${PLURAL[r.kind]}/${r.id}${path}`

type Item = NavMenu["groups"][number]["items"][number]

/** Menu latéral d'une ressource, organisé comme les configuration-sidebar du fork. */
export function resourceMenu(r: ResourceRef): NavMenu {
  const at = (path: string) => resourcePage(r, path)
  const item = (label: string, icon: Item["icon"], path: string, extra: Partial<Item> = {}): Item => ({ label, icon, to: at(path), ...extra })
  const general = item("Général", SettingsIcon, "", { exact: true })
  const envs = item("Variables d'environnement", VariableIcon, "/envs")
  const storages = item("Stockage persistant", HardDriveIcon, "/storages")
  const logs = item("Logs", ListIcon, "/logs")
  const terminal = item("Terminal", SquareTerminalIcon, "/terminal")
  const scheduled = item("Tâches planifiées", CalendarClockIcon, "/scheduled-tasks")
  const operations = item("Opérations", ServerCogIcon, "/operations")
  const tags = item("Tags", TagIcon, "/tags")
  const danger = item("Zone de danger", ShieldAlertIcon, "/danger", { danger: true })

  if (r.kind === "service") {
    return {
      label: "Service",
      groups: [
        { items: [general, item("Domaines", GlobeIcon, "/domains"), envs] },
        { items: [logs, terminal, item("Historique", HistoryIcon, "/tasks")] },
        { items: [scheduled, item("Webhooks", WebhookIcon, "/webhooks"), operations, tags] },
        { items: [danger] },
      ],
    }
  }
  if (r.kind === "database") {
    return {
      label: "Base de données",
      groups: [
        { items: [general, envs, storages, item("Sauvegardes", DatabaseBackupIcon, "/backups"), item("Importer un dump", UploadIcon, "/import")] },
        { items: [logs, terminal, item("Historique", HistoryIcon, "/tasks")] },
        { items: [scheduled, item("Webhooks", WebhookIcon, "/webhooks"), operations, tags] },
        { items: [danger] },
      ],
    }
  }
  return {
    label: "Application",
    groups: [
      { items: [general, item("Avancé", SlidersHorizontalIcon, "/advanced"), envs, storages, item("Source Git", GitBranchIcon, "/source")] },
      { items: [item("Déploiements", RocketIcon, "/deployments"), logs, terminal] },
      {
        items: [
          item("Previews", GitPullRequestIcon, "/previews"),
          item("Webhooks", WebhookIcon, "/webhooks"),
          scheduled,
          item("Healthcheck", ActivityIcon, "/healthcheck"),
          item("Limites", BoxIcon, "/limits"),
          item("Compose", CodeXmlIcon, "/compose"),
          operations,
          tags,
        ],
      },
      { items: [danger] },
    ],
  }
}
