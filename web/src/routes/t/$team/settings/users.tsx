import { createFileRoute } from "@tanstack/react-router"
import { AdminUsersPage } from "@/features/admin/admin"

export const Route = createFileRoute("/t/$team/settings/users")({ component: AdminUsersPage })
