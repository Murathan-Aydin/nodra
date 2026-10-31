import { BoxIcon, ContainerIcon, DatabaseIcon, FileCode2Icon, GitBranchIcon, KeyRoundIcon, LayersIcon, PackageIcon, type LucideIcon } from "lucide-react"

export type Kind =
  | "public"
  | "deploy-key"
  | "github"
  | "gitlab"
  | "gitea"
  | "dockerfile"
  | "compose"
  | "image"
  | "postgresql"
  | "mysql"
  | "mariadb"
  | "redis"
  | "keydb"
  | "dragonfly"
  | "mongodb"
  | "clickhouse"

export type Option = { id: Kind; name: string; description: string; icon: LucideIcon; tint: string; group: "git" | "docker" | "database" }

// Mêmes choix que l'écran « Choose a resource » de Coolify (SQLite non pris en charge par Nodra).
export const APPLICATIONS: Option[] = [
  { id: "public", group: "git", name: "Dépôt Git public", description: "Déployer n'importe quel dépôt public. Nodra le construit depuis les sources, sans identifiants.", icon: GitBranchIcon, tint: "text-orange-400" },
  { id: "deploy-key", group: "git", name: "Dépôt Git privé (clé de déploiement)", description: "Dépôt privé en SSH avec une clé de déploiement propre au dépôt. URL et branche à la main, sans déploiement automatique.", icon: KeyRoundIcon, tint: "text-amber-400" },
  { id: "github", group: "git", name: "Dépôt Git (GitHub App)", description: "Dépôts GitHub publics ou privés via une GitHub App : webhooks automatiques et previews de pull requests.", icon: GitBranchIcon, tint: "text-zinc-200" },
  { id: "gitlab", group: "git", name: "Dépôt Git (GitLab)", description: "Projets GitLab publics ou privés : webhooks automatiques et previews de merge requests.", icon: GitBranchIcon, tint: "text-orange-500" },
  { id: "gitea", group: "git", name: "Dépôt Git (Gitea / Forgejo)", description: "Dépôts Gitea publics ou privés : webhooks automatiques et previews de pull requests.", icon: GitBranchIcon, tint: "text-green-500" },
  { id: "dockerfile", group: "docker", name: "Dockerfile", description: "Déployer une application décrite par un Dockerfile, sans dépôt Git.", icon: FileCode2Icon, tint: "text-sky-400" },
  { id: "compose", group: "docker", name: "Docker Compose", description: "Déployer une application multi-conteneurs décrite par un fichier Docker Compose, sans dépôt Git.", icon: LayersIcon, tint: "text-sky-400" },
  { id: "image", group: "docker", name: "Image Docker", description: "Déployer une image déjà construite, depuis n'importe quel registre, sans dépôt Git.", icon: PackageIcon, tint: "text-sky-400" },
]

export const DATABASES: Option[] = [
  { id: "postgresql", group: "database", name: "PostgreSQL", description: "Base relationnelle respectueuse des standards SQL et très extensible.", icon: DatabaseIcon, tint: "text-sky-400" },
  { id: "mysql", group: "database", name: "MySQL", description: "Base relationnelle pour le web et les usages généraux.", icon: DatabaseIcon, tint: "text-blue-400" },
  { id: "mariadb", group: "database", name: "MariaDB", description: "Base relationnelle, remplaçante directe de MySQL.", icon: DatabaseIcon, tint: "text-amber-500" },
  { id: "redis", group: "database", name: "Redis", description: "Stockage clé-valeur en mémoire : base, cache et courtier de messages.", icon: DatabaseIcon, tint: "text-red-500" },
  { id: "keydb", group: "database", name: "KeyDB", description: "Stockage en mémoire multithread, compatible Redis.", icon: DatabaseIcon, tint: "text-rose-400" },
  { id: "dragonfly", group: "database", name: "Dragonfly", description: "Stockage en mémoire compatible Redis et Memcached.", icon: DatabaseIcon, tint: "text-violet-400" },
  { id: "mongodb", group: "database", name: "MongoDB", description: "Base NoSQL orientée documents (de type JSON).", icon: DatabaseIcon, tint: "text-green-500" },
  { id: "clickhouse", group: "database", name: "ClickHouse", description: "Base en colonnes pour l'analytique en temps réel sur de gros volumes.", icon: DatabaseIcon, tint: "text-yellow-400" },
]

export const ALL_OPTIONS = [...APPLICATIONS, ...DATABASES]
export const SERVICE_ICON = BoxIcon
export const DOCKER_ICON = ContainerIcon
export const isDatabase = (k: string) => DATABASES.some((d) => d.id === k)
