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
