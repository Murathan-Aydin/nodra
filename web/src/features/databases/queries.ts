import { queryOptions } from "@tanstack/react-query"
import { get, teamPath } from "@/lib/api"
import type { Database, ServerTask, Tag } from "@/lib/types"

export const dbBase = (team: string, db: string) => teamPath(team, `/databases/${db}`)

export const dbQuery = (team: string, db: string) =>
  queryOptions({
    queryKey: ["team", team, "db", db],
    queryFn: () => get<{ database: Database; tags: Tag[]; server_id: string; project_id: string }>(dbBase(team, db)),
  })

export const dbTasksQuery = (team: string, db: string) =>
  queryOptions({ queryKey: ["team", team, "db", db, "tasks"], queryFn: () => get<ServerTask[]>(`${dbBase(team, db)}/tasks`) })

type DBType = {
  label: string
  image: string
  port: number
  /** Identifiant par défaut ; absent si non configurable (Redis, KeyDB, Dragonfly). */
  username?: string
  /** Nom de base par défaut, si le type en a un. */
  database?: string
  rootPassword?: boolean
  /** Modes TLS proposés ; `[]` = TLS sans mode ; absent = TLS non pris en charge. */
  sslModes?: string[]
  /** Fichier de configuration personnalisée. */
  config?: string
  initScripts?: boolean
  /** Sauvegarde et restauration (import) prises en charge. */
  backup?: boolean
  restore?: boolean
  importExts?: string
}

export const DB_TYPES: Record<Database["type"], DBType> = {
  postgresql: {
    label: "PostgreSQL",
    image: "postgres:17-alpine",
    port: 5432,
    username: "postgres",
    database: "postgres",
    sslModes: ["require", "verify-ca", "verify-full", "prefer", "allow"],
    config: "postgresql.conf",
    initScripts: true,
    backup: true,
    restore: true,
    importExts: ".dump,.sql,.sql.gz,.gz",
  },
  mysql: {
    label: "MySQL",
    image: "mysql:8",
    port: 3306,
    username: "mysql",
    database: "default",
    rootPassword: true,
    sslModes: [],
    config: "my.cnf",
    initScripts: true,
    backup: true,
    restore: true,
    importExts: ".sql,.sql.gz,.gz",
  },
  mariadb: {
    label: "MariaDB",
    image: "mariadb:11",
    port: 3306,
    username: "mariadb",
    database: "default",
    rootPassword: true,
    sslModes: [],
    config: "my.cnf",
    initScripts: true,
    backup: true,
    restore: true,
    importExts: ".sql,.sql.gz,.gz",
  },
  mongodb: {
    label: "MongoDB",
    image: "mongo:7",
    port: 27017,
    username: "root",
    database: "default",
    sslModes: ["requireTLS", "preferTLS", "allowTLS"],
    config: "mongod.conf",
    initScripts: true,
    backup: true,
    restore: true,
    importExts: ".archive,.archive.gz,.gz",
  },
  redis: { label: "Redis", image: "redis:7.2", port: 6379, sslModes: [], config: "redis.conf", backup: true },
  keydb: { label: "KeyDB", image: "eqalpha/keydb:latest", port: 6379, sslModes: [], config: "keydb.conf", backup: true },
  dragonfly: { label: "Dragonfly", image: "docker.dragonflydb.io/dragonflydb/dragonfly", port: 6379 },
  clickhouse: { label: "ClickHouse", image: "clickhouse/clickhouse-server:25.8", port: 9000, username: "default", database: "default" },
}
