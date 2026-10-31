import { useQueryClient } from "@tanstack/react-query"
import { useRef } from "react"
import { useSocket } from "@/lib/ws"
import { teamPath } from "@/lib/api"
import type { TeamEvent } from "@/lib/types"

// Événements trop fréquents, suivis par leurs propres flux (logs de déploiement, sortie de tâche).
const IGNORED = new Set(["deployment.log", "task.output"])
// Requêtes d'équipe jamais concernées par un changement d'état.
const STABLE = new Set(["access", "permissions", "catalog"])

/**
 * Écoute les événements de l'équipe et rafraîchit les données affichées (regroupés sur 400 ms).
 * Les écouteurs passés reçoivent aussi chaque événement (toasts, compteurs).
 */
export function useTeamEvents(team: string, onEvent?: (e: TeamEvent) => void) {
  const qc = useQueryClient()
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  return useSocket(teamPath(team, "/events"), undefined, {
    onMessage: (msg) => {
      if (msg.type !== "event") return
      const event = msg.event as TeamEvent
      onEvent?.(event)
      if (IGNORED.has(event.kind)) return
      clearTimeout(timer.current)
      timer.current = setTimeout(() => {
        void qc.invalidateQueries({
          predicate: (q) => q.queryKey[0] === "team" && q.queryKey[1] === team && !STABLE.has(String(q.queryKey[2])),
        })
      }, 400)
    },
  })
}
