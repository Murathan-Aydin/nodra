import { createFileRoute } from "@tanstack/react-router"
import { AdminGeneralPage } from "@/features/admin/admin"

export const Route = createFileRoute("/t/$team/settings/")({ component: AdminGeneralPage })
