import { createFileRoute } from "@tanstack/react-router"
import { AdminAdvancedPage } from "@/features/admin/admin"

export const Route = createFileRoute("/t/$team/settings/advanced")({ component: AdminAdvancedPage })
