import { queryOptions } from "@tanstack/react-query"
import { get } from "@/lib/api"
import type { Container } from "@/lib/types"

export const containersQuery = (team: string, resource: string, base: string, pr = 0) =>
  queryOptions({
    queryKey: ["team", team, "containers", resource, pr],
    queryFn: () => get<Container[]>(`${base}/containers`, { pr: pr || undefined }),
    retry: false,
  })
