// Objets renvoyés par l'API (voir docs/API.md).

export interface Instance {
  name: string
  version: string
  registration_enabled: boolean
  oauth_providers: { provider: string; name: string }[]
  /** Aucun compte : le premier inscrit devient administrateur. */
  needs_setup: boolean
  /** Configuration guidée encore proposée (une seule fois, après le premier compte). */
  onboarding: boolean
  /** install.sh a préparé l'accès SSH à l'hôte (« Cette machine »). */
  localhost: boolean
}

export interface User {
  id: string
  email: string
  name: string
  is_instance_admin: boolean
  email_verified_at: string | null
}

export interface Team {
  id: string
  parent_id: string | null
  name: string
  personal: boolean
}

export interface PrivateKey {
  id: string
  name: string
  description?: string
  public_key: string
  fingerprint: string
  created_at?: string
}

export interface Server {
  id: string
  name: string
  description: string
  ip: string
  port: number
  user: string
  is_reachable: boolean
  is_usable: boolean
  is_localhost: boolean
  os_id: string
  os_version: string
  docker_version: string
  compose_version?: string
  validation_error: string
  validated_at: string | null
  private_key_id?: string
  host_key?: string
  last_seen_at?: string | null
  last_cleanup_at?: string | null
  settings?: Record<string, unknown>
  team_id?: string
}

export interface ServerTask {
  id: string
  server_id: string
  kind: string
  status: "queued" | "running" | "success" | "failed" | "cancelled"
  output?: string
  error: string
  created_at: string
  started_at?: string | null
  finished_at?: string | null
}

export interface Project {
  id: string
  name: string
}

export interface ProjectSummary extends Project {
  description: string
  environment_count: number
}

export interface ResourceSummary {
  id: string
  type: "application" | "database" | "service"
  name: string
  status: string
  environment_id: string
  environment_name: string
  project_id: string
  project_name: string
  server_id: string
  domains: string[] | null
  tags?: { id: string; name: string; color: string }[] | null
}

export interface Deployment {
  id: string
  application_id: string
  server_id: string
  status: "queued" | "in_progress" | "finished" | "failed" | "cancelled"
  trigger?: "manual" | "api" | "webhook" | "rollback" | "restart" | "approval" | "preview"
  branch?: string
  commit_sha?: string
  commit_message?: string
  pr_number: number
  force_rebuild?: boolean
  image?: string
  error?: string
  cancel_requested?: boolean
  created_at: string
  started_at?: string | null
  finished_at?: string | null
}

export interface ServerMetric {
  ts: string
  cpu_percent: number
  mem_used: number
  mem_total: number
}

export interface Environment {
  id: string
  project_id: string
  name: string
  description: string
  created_at: string
}

export interface Destination {
  id: string
  server_id: string
  name: string
  network: string
  is_default: boolean
}

export interface GitSource {
  id: string
  type: "github" | "gitlab" | "gitea"
  name: string
  is_public: boolean
}

export interface Repo {
  full_name: string
  name: string
  private: boolean
  default_branch: string
}

export interface ServiceTemplate {
  slug: string
  name: string
  slogan: string
  category: string
  tags: string[] | null
}

export interface Service {
  id: string
  environment_id: string
  name: string
  description: string
  template: string
  docker_compose_raw: string
  status: string
}

export interface ServiceApp {
  id: string
  name: string
  image: string
  domains: string[] | null
  port: string
  exclude_from_status: boolean
}

export interface Container {
  name: string
  service: string
  state: string
  status: string
  image: string
}

export interface EnvVar {
  id: string
  key: string
  value: string
  is_secret: boolean
  is_literal: boolean
  is_multiline: boolean
  comment: string
}

export interface Tag {
  id: string
  name: string
  color: string
}

export interface Database {
  id: string
  environment_id: string
  type: "postgresql" | "mysql" | "mariadb" | "mongodb" | "redis" | "keydb" | "dragonfly" | "clickhouse"
  name: string
  description: string
  image: string
  username: string
  password?: string
  root_password?: string
  database_name: string
  custom_config: string
  init_scripts: { filename: string; content: string }[] | null
  is_public: boolean
  public_port: number | null
  ports_mappings: string
  custom_docker_run_options: string
  limits: Record<string, string | number>
  settings: Record<string, unknown>
  status: string
  internal_host: string
  internal_port: number
  internal_url?: string
  public_url?: string
  ca_certificate?: string
  started_at: string | null
  created_at: string
}

export interface HealthCheck {
  enabled: boolean
  type: "http" | "cmd"
  path: string
  port: string
  host: string
  method: string
  return_code: number
  scheme: string
  response_text: string
  interval: number
  timeout: number
  retries: number
  start_period: number
  command: string
}

export type BuildPack = "nixpacks" | "dockerfile" | "dockercompose" | "dockerimage" | "static"

export interface Application {
  id: string
  environment_id: string
  destination_id: string
  name: string
  description: string
  status: string
  git_repository: string
  git_branch: string
  git_commit_sha: string
  source_id: string | null
  private_key_id: string | null
  build_pack: BuildPack
  docker_image: string
  docker_image_tag: string
  static_image: string
  base_directory: string
  publish_directory: string
  dockerfile: string
  dockerfile_location: string
  dockerfile_target: string
  docker_compose_location: string
  docker_compose_raw: string
  docker_compose_domains: Record<string, string[]> | null
  install_command: string
  build_command: string
  start_command: string
  watch_paths: string
  ports_exposes: string
  ports_mappings: string
  domains: string[] | null
  redirect: "both" | "www" | "non-www"
  preview_url_template: string
  custom_labels: string
  custom_docker_run_options: string
  pre_deployment_command: string
  pre_deployment_container: string
  post_deployment_command: string
  post_deployment_container: string
  health_check: HealthCheck
  limits: Record<string, string | number>
  settings: Record<string, unknown>
  basic_auth_password_set?: boolean
  created_at: string
}

export interface DeploymentLog {
  id: number
  created_at: string
  kind: string
  hidden: boolean
  message: string
}

export interface Preview {
  id: string
  application_id: string
  pr_number: number
  pr_url: string
  title: string
  domains: string[] | null
  status: "stopped" | "running"
  last_commit: string
  created_at: string
}

