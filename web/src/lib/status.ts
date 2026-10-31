// Statut d'une ressource (« running:healthy », « exited », « stopped »…) → libellé et couleur.

export function resourceStatus(status: string): { label: string; dot: string } {
  const [state, health] = status.split(":")
  switch (state) {
    case "running":
      return health === "unhealthy" ? { label: "En cours (dégradé)", dot: "bg-warning" } : { label: "En cours", dot: "bg-success" }
    case "degraded":
      return { label: "Dégradé", dot: "bg-warning" }
    case "restarting":
    case "starting":
      return { label: "Redémarrage", dot: "bg-primary animate-pulse" }
    case "exited":
      return { label: "Arrêté (erreur)", dot: "bg-destructive" }
    default:
      return { label: "Arrêté", dot: "bg-muted-foreground" }
  }
}
