import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api } from "@/lib/api"
import { resourceApi, resourceKey, type ResourceRef } from "./resource"

/** Enregistrement partiel d'une ressource (PATCH), avec message et rafraîchissement. */
export function useResourcePatch(resource: ResourceRef, message = "Modifications enregistrées") {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (fields: Record<string, unknown>) => api(resourceApi(resource), { method: "PATCH", json: fields }),
    onSuccess: () => {
      toast.success(message)
      void qc.invalidateQueries({ queryKey: resourceKey(resource) })
      void qc.invalidateQueries({ queryKey: ["team", resource.team, "resources"] })
    },
    onError: (e) => toast.error(e.message),
  })
}

/** Brouillon d'un formulaire : valeurs, mise à jour d'un champ, champs modifiés. */
export function useDraft<T extends Record<string, unknown>>(initial: T) {
  const [draft, setDraft] = useState(initial)
  const changed = Object.fromEntries(Object.entries(draft).filter(([k, v]) => JSON.stringify(v) !== JSON.stringify(initial[k]))) as Partial<T>
  const set = <K extends keyof T>(key: K, value: T[K]) => setDraft((d) => ({ ...d, [key]: value }))
  return { draft, set, changed, dirty: Object.keys(changed).length > 0 }
}
