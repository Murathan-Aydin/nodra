import { createFileRoute } from "@tanstack/react-router"
import { AdminEmailPage } from "@/features/admin/admin"

export const Route = createFileRoute("/t/$team/settings/email")({ component: AdminEmailPage })
