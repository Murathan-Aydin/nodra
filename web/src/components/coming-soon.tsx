import { ConstructionIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"

/** Page dont l'interface n'est pas encore écrite (l'API existe déjà : voir docs/API.md). */
export function ComingSoon({ title, card }: { title: string; card: string }) {
  return (
    <>
      <PageHeader title={title} />
      <EmptyState icon={ConstructionIcon} title="Bientôt disponible" description={`Cette page arrive avec la carte ${card} du kanban du front.`} />
    </>
  )
}
