import { createFileRoute } from "@tanstack/react-router"
import { AccountPage } from "@/features/account/account"

export const Route = createFileRoute("/t/$team/account/")({ component: AccountPage })
