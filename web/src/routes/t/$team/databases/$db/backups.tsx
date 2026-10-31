import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { usePermission } from "@/lib/permissions"
import { BackupsPanel } from "@/features/backups/backups-panel"
import { ImportDump } from "@/features/databases/import-dump"
import { DB_TYPES, dbQuery } from "@/features/databases/queries"

export const Route = createFileRoute("/t/$team/databases/$db/backups")({ component: DatabaseBackups })

function DatabaseBackups() {
  const { team, db } = Route.useParams()
  const { data } = useQuery(dbQuery(team, db))
  const canRestore = usePermission(team, "resource.backup.restore", { type: "resource", id: db })
  if (!data) return null
  const t = DB_TYPES[data.database.type]
  return (
    <div className="space-y-6">
      {!t.backup && <p className="text-sm text-muted-foreground">Pas de dump pour {t.label} : seules les archives de volumes sont possibles.</p>}
      {t.backup && !t.restore && <p className="text-sm text-muted-foreground">{t.label} : restauration manuelle (remplacer le fichier RDB base arrêtée).</p>}
      <BackupsPanel team={team} resource={db} database={!!t.backup} restorable={!!t.restore} dumpAll={["postgresql", "mysql", "mariadb"].includes(data.database.type)} />
      {t.restore && canRestore && <ImportDump team={team} db={db} accept={t.importExts ?? ""} />}
    </div>
  )
}
