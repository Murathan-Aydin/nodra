import { createFileRoute } from "@tanstack/react-router"
import { AdminAuditPage } from "@/features/admin/admin"

export const Route = createFileRoute("/t/$team/settings/audit")({ component: AdminAuditPage })
