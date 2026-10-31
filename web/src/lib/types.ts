// Objets renvoyés par l'API (voir docs/API.md).

export interface Instance {
  name: string
  version: string
  registration_enabled: boolean
  oauth_providers: { provider: string; name: string }[]
  /** Aucun compte : le premier inscrit devient administrateur. */
  needs_setup: boolean
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
