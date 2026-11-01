import { createFileRoute } from "@tanstack/react-router"
import { AdminOAuthPage } from "@/features/admin/admin"

export const Route = createFileRoute("/t/$team/settings/oauth")({ component: AdminOAuthPage })
