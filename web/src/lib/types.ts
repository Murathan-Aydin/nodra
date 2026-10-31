// Types des objets de l'API (voir docs/API.md).

export interface User {
  id: string
  email: string
  name: string
  is_instance_admin: boolean
  email_verified_at: string | null
  disabled_at?: string | null
  created_at: string
}

export interface Me {
  user: User
  two_factor_enabled: boolean
}

export interface OAuthProvider {
  provider: string
  name: string
}

export interface Instance {
  name: string
  registration_enabled: boolean
  version: string
  oauth_providers: OAuthProvider[]
}

export interface Team {
  id: string
  parent_id: string | null
  name: string
  description: string
  personal: boolean
  created_at: string
}

export interface TeamAccess {
  team: Team
  ancestors: string[]
  permissions: string[]
  is_owner: boolean
}

export interface Permission {
  key: string
  group: string
  description: string
}

export interface PermissionCatalog {
  permissions: Permission[]
  scope_types: string[]
  token_scopes: string[]
  gateable_actions: string[]
}

export interface Scope {
  type: string
  id: string
}

export interface ApprovalVote {
  user_id: string
  email: string
  decision: "approve" | "reject"
  comment?: string
  created_at: string
}

export interface ApprovalRequest {
  id: string
  team_id: string
  rule_id: string | null
  required_approvals: number
  action: string
  scope: Scope
  summary: string
  payload: unknown
  requested_by: string | null
  status: "pending" | "approved" | "rejected" | "cancelled" | "expired" | "executed" | "failed" | string
  error: string
  expires_at: string
  decided_at: string | null
  created_at: string
  votes: ApprovalVote[]
}

export interface ServerTask {
  id: string
  team_id: string
  server_id: string
  kind: string
  params: Record<string, unknown>
  status: "queued" | "running" | "success" | "failed" | "cancelled"
  output?: string
  error: string
  created_by: string | null
  created_at: string
  started_at: string | null
  finished_at: string | null
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
  domains: string[]
  tags: string[]
  created_at: string
}

export interface Deployment {
  id: string
  team_id: string
  application_id: string
  server_id: string
  status: "queued" | "in_progress" | "finished" | "failed" | "cancelled"
  trigger: string
  branch: string
  commit_sha: string
  commit_message: string
  pr_number: number
  force_rebuild: boolean
  rollback_image: string
  image: string
  error: string
  created_at: string
  started_at: string | null
  finished_at: string | null
}

export interface ServerSummary {
  id: string
  team_id: string
  name: string
  description: string
  ip: string
  port: number
  user: string
  is_reachable: boolean
  is_usable: boolean
  validation_error: string
  docker_version: string
  last_seen_at: string | null
  proxy: { type: string; status: string; acme_email: string }
  created_at: string
}

/** Réponse 202 d'une action soumise aux approbations. */
export interface Pending {
  approval_request?: ApprovalRequest
}

/** Événement temps réel (GET /teams/{team}/events). */
export interface TeamEvent {
  team: string
  kind: string
  id: string
  resource?: string
  server?: string
  status?: string
}

export interface ServerSettings {
  concurrent_builds: number
  deployment_timeout: number
  is_build_server: boolean
  docker_cleanup_frequency: string
  docker_cleanup_threshold: number
  delete_unused_volumes: boolean
  delete_unused_networks: boolean
  metrics_enabled: boolean
  metrics_retention_days: number
  timezone: string
  wildcard_domain: string
  connection_timeout: number
  cloudflare_tunnel: boolean
}

export interface Server extends ServerSummary {
  private_key_id: string
  host_key: string
  os_id: string
  os_version: string
  compose_version: string
  validated_at: string | null
  last_cleanup_at: string | null
  settings: ServerSettings
}

export interface Destination {
  id: string
  server_id: string
  name: string
  network: string
  is_default: boolean
  created_at: string
}

export interface PrivateKey {
  id: string
  team_id: string
  name: string
  description: string
  public_key: string
  fingerprint: string
  created_at: string
}

export interface ServerMetric {
  ts: string
  cpu_percent: number
  mem_used: number
  mem_total: number
  disk_used: number
  disk_total: number
  load1: number
}

export interface ProxyState {
  type: "traefik" | "caddy" | "none"
  status: string
  acme_email: string
  compose: string
  is_custom: boolean
  default_compose: string
}

export interface ProxyConfig {
  id: string
  server_id: string
  name: string
  content: string
  updated_at: string
}

export interface Certificate {
  id: string
  name: string
  domains: string[]
  certificate: string
  expires_at: string
}

export interface Project {
  id: string
  team_id: string
  name: string
  description: string
  created_at: string
}

export interface Environment {
  id: string
  project_id: string
  name: string
  description: string
  created_at: string
}

export interface SharedVariable {
  id: string
  team_id: string
  scope: Scope
  key: string
  value: string
  comment: string
  is_secret: boolean
  is_multiline: boolean
  is_literal: boolean
  updated_at: string
}

export interface Tag {
  id: string
  team_id: string
  name: string
  color: string
  created_at: string
}

export interface GitSource {
  id: string
  team_id: string
  type: "github" | "gitlab" | "gitea"
  name: string
  html_url: string
  api_url: string
  organization: string
  is_public: boolean
  app_id: number
  app_slug: string
  client_id: string
  installation_id: number
  configured: boolean
  installed: boolean
  has_private_key: boolean
  has_webhook_secret: boolean
  has_access_token: boolean
  created_at: string
}

export interface Repo {
  full_name: string
  name: string
  private: boolean
  default_branch: string
  html_url: string
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

export interface Limits {
  memory: string
  memory_swap: string
  memory_reservation: string
  memory_swappiness: number
  cpus: string
  cpuset: string
  cpu_shares: number
}

export interface Application {
  id: string
  team_id: string
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
  build_pack: "nixpacks" | "dockerfile" | "dockercompose" | "dockerimage" | "static"
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
  docker_compose_domains: Record<string, string[]>
  install_command: string
  build_command: string
  start_command: string
  watch_paths: string
  ports_exposes: string
  ports_mappings: string
  domains: string[]
  redirect: "both" | "www" | "non-www"
  preview_url_template: string
  custom_labels: string
  custom_docker_run_options: string
  pre_deployment_command: string
  pre_deployment_container: string
  post_deployment_command: string
  post_deployment_container: string
  health_check: HealthCheck
  limits: Limits
  settings: Record<string, unknown> & { log_drain_enabled?: boolean; auto_deploy?: boolean; preview_deployments?: boolean }
  basic_auth_password_set: boolean
  created_at: string
  updated_at: string
}

export interface EnvVar {
  id: string
  resource_type: string
  resource_id: string
  key: string
  value: string
  is_buildtime: boolean
  is_runtime: boolean
  is_preview: boolean
  is_literal: boolean
  is_multiline: boolean
  is_secret: boolean
  comment: string
  updated_at: string
}

export interface Storage {
  id: string
  resource_type: string
  resource_id: string
  kind: "volume" | "bind" | "file"
  name: string
  host_path: string
  mount_path: string
  content: string
  created_at: string
}

export interface Container {
  name: string
  service?: string
  state: string
  status: string
  image: string
}

export interface Preview {
  id: string
  application_id: string
  pr_number: number
  pr_url: string
  title: string
  domains: string[]
  compose_domains: Record<string, string[]>
  status: string
  last_commit: string
  created_at: string
  updated_at: string
}

export interface DeploymentLog {
  id: number
  created_at: string
  kind: string
  hidden: boolean
  message: string
}

export interface ImageInfo {
  tag: string
  image: string
  created: string
  size: string
  current: boolean
}

export interface ScheduledTask {
  id: string
  team_id: string
  resource_id: string
  name: string
  command: string
  frequency: string
  container: string
  timeout: number
  enabled: boolean
  last_run_at: string | null
  created_at: string
}

export interface InitScript {
  filename: string
  content: string
}

export interface Database {
  id: string
  team_id: string
  environment_id: string
  destination_id: string
  type: "postgresql" | "mysql" | "mariadb" | "mongodb" | "redis" | "keydb" | "dragonfly" | "clickhouse"
  name: string
  description: string
  image: string
  status: string
  username: string
  database_name: string
  custom_config: string
  init_scripts: InitScript[]
  is_public: boolean
  public_port: number | null
  ports_mappings: string
  custom_docker_run_options: string
  limits: Limits
  settings: {
    health_check_enabled: boolean
    stop_grace_period: number
    postgres_initdb_args: string
    postgres_host_auth_method: string
    log_drain_enabled: boolean
    enable_ssl: boolean
    ssl_mode: string
  }
  started_at: string | null
  created_at: string
  updated_at: string
  internal_host: string
  internal_port: number
  ca_certificate?: string
  // Présents avec resource.env.read uniquement.
  password?: string
  root_password?: string
  internal_url?: string
  public_url?: string
}

export interface S3Storage {
  id: string
  team_id: string
  name: string
  description: string
  endpoint: string
  region: string
  bucket: string
  path_prefix: string
  is_usable: boolean
  last_error: string
  checked_at: string | null
  created_at: string
}

export interface BackupSchedule {
  id: string
  team_id: string
  kind: "database" | "volume"
  resource_id: string
  volume_name: string
  databases: string
  dump_all: boolean
  frequency: string
  enabled: boolean
  keep_local: number
  keep_local_days: number
  local_backup: boolean
  s3_storage_id: string | null
  keep_s3: number
  keep_s3_days: number
  last_run_at: string | null
  created_at: string
}

export interface BackupExecution {
  id: string
  schedule_id: string | null
  team_id: string
  resource_id: string
  server_id: string
  database_name: string
  status: string
  filename: string
  size: number
  s3_storage_id: string | null
  s3_key: string
  local_deleted: boolean
  s3_deleted: boolean
  message: string
  created_at: string
  finished_at: string | null
}

export interface ServiceTemplate {
  slug: string
  name: string
  slogan: string
  documentation: string
  category: string
  tags: string[] | null
  logo: string
  port: string
  compose?: string
}

export interface Service {
  id: string
  team_id: string
  environment_id: string
  destination_id: string
  name: string
  description: string
  template: string
  docker_compose_raw: string
  status: string
  log_drain_enabled: boolean
  created_at: string
  updated_at: string
}

export interface ServiceApp {
  id: string
  service_id: string
  name: string
  image: string
  domains: string[]
  port: string
  url_var: string
  exclude_from_status: boolean
}
