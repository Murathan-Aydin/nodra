import { redirect } from "@tanstack/react-router"
import type { QueryClient } from "@tanstack/react-query"
import { ApiError } from "./api"
import { instanceQuery, meQuery } from "./queries"

/** Instance sans compte : tout mène à la création du compte administrateur. */
export async function requireSetupDone(qc: QueryClient) {
  const instance = await qc.ensureQueryData(instanceQuery)
  if (instance.needs_setup) throw redirect({ to: "/setup" })
  return instance
}

/** Page connectée : renvoie vers la connexion (puis ici) sans session. */
export async function requireSession(qc: QueryClient, from: string) {
  await requireSetupDone(qc)
  try {
    return await qc.ensureQueryData(meQuery)
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) throw redirect({ to: "/login", search: { redirect: from } })
    throw e
  }
}
