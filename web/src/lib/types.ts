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
  public_key: string
  fingerprint: string
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
  validation_error: string
  validated_at: string | null
}

export interface ServerTask {
  id: string
  server_id: string
  kind: string
  status: "queued" | "running" | "success" | "failed" | "cancelled"
  output?: string
  error: string
  created_at: string
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
}

export interface Deployment {
  id: string
  application_id: string
  server_id: string
  status: "queued" | "in_progress" | "finished" | "failed" | "cancelled"
  pr_number: number
  created_at: string
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
