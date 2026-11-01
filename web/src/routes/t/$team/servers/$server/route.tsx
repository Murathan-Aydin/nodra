import { Outlet, createFileRoute } from "@tanstack/react-router"
import { serverMenu } from "@/features/servers/server"

// Pages d'un serveur : le menu du serveur remplace le menu principal (comme le fork).
export const Route = createFileRoute("/t/$team/servers/$server")({
  staticData: { nav: (p) => serverMenu(p.team, p.server) },
  component: Outlet,
})
