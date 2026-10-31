import { queryOptions } from "@tanstack/react-query"
import { get, teamPath } from "@/lib/api"
import type { ApprovalRequest, ApprovalRule, Group, Invitation, Member, Role, RoleAssignment } from "@/lib/types"

export const membersQuery = (team: string) => queryOptions({ queryKey: ["team", team, "members"], queryFn: () => get<Member[]>(teamPath(team, "/members")) })

export const invitationsQuery = (team: string) =>
  queryOptions({ queryKey: ["team", team, "invitations"], queryFn: () => get<Invitation[]>(teamPath(team, "/invitations")), retry: false })

export const rolesQuery = (team: string) => queryOptions({ queryKey: ["team", team, "roles"], queryFn: () => get<Role[]>(teamPath(team, "/roles")) })

export const assignmentsQuery = (team: string) =>
  queryOptions({ queryKey: ["team", team, "role-assignments"], queryFn: () => get<RoleAssignment[]>(teamPath(team, "/role-assignments")) })

export const groupsQuery = (team: string) => queryOptions({ queryKey: ["team", team, "groups"], queryFn: () => get<Group[]>(teamPath(team, "/groups")) })

export const rulesQuery = (team: string) => queryOptions({ queryKey: ["team", team, "approval-rules"], queryFn: () => get<ApprovalRule[]>(teamPath(team, "/approval-rules")) })

export const approvalsQuery = (team: string, status: string) =>
  queryOptions({
    queryKey: ["team", team, "approvals", status],
    queryFn: () => get<ApprovalRequest[]>(teamPath(team, "/approvals"), { status: status === "all" ? undefined : status }),
  })
