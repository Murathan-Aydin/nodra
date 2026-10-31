import { useMutation, useQueryClient, type QueryKey } from "@tanstack/react-query"
import { toast } from "sonner"
import { ApiError } from "./api"
import type { ApprovalRequest } from "./types"

/** Signale une demande d'approbation créée à la place de l'action (réponse 202). */
export function notifyPending(res: unknown): boolean {
  const req = (res as { approval_request?: ApprovalRequest } | undefined)?.approval_request
  if (!req) return false
  toast.info("Demande d'approbation envoyée", {
    description: `${req.summary} — ${req.required_approvals} approbation(s) requise(s).`,
  })
  return true
}

/**
 * Mutation de l'API avec notifications : erreur en toast, succès (`success`) ou demande d'approbation,
 * puis invalidation des requêtes indiquées.
 */
export function useApiMutation<TVars, TRes>(
  fn: (vars: TVars) => Promise<TRes>,
  opts: { success?: string | ((res: TRes, vars: TVars) => string | undefined); invalidate?: QueryKey[]; onSuccess?: (res: TRes, vars: TVars) => void } = {},
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: (res, vars) => {
      if (!notifyPending(res)) {
        const msg = typeof opts.success === "function" ? opts.success(res, vars) : opts.success
        if (msg) toast.success(msg)
      }
      for (const key of opts.invalidate ?? []) qc.invalidateQueries({ queryKey: key })
      opts.onSuccess?.(res, vars)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "Erreur inattendue")
    },
  })
}
