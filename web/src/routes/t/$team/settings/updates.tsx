import { createFileRoute } from "@tanstack/react-router"
import { AdminUpdatesPage } from "@/features/admin/admin"

export const Route = createFileRoute("/t/$team/settings/updates")({ component: AdminUpdatesPage })
