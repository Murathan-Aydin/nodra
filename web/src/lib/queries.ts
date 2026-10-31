import { queryOptions } from "@tanstack/react-query"
import { get } from "./api"
import type { Instance, Team, User } from "./types"

export const instanceQuery = queryOptions({ queryKey: ["instance"], queryFn: () => get<Instance>("/instance") })

export const meQuery = queryOptions({
  queryKey: ["me"],
  queryFn: () => get<{ user: User; two_factor_enabled: boolean }>("/me"),
  retry: false,
})

export const teamsQuery = queryOptions({ queryKey: ["teams"], queryFn: () => get<Team[]>("/teams") })
