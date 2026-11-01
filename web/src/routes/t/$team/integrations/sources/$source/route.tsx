import { Outlet, createFileRoute } from "@tanstack/react-router"
import { LayoutGridIcon, KeyRoundIcon, SettingsIcon, ShieldAlertIcon } from "lucide-react"

// Pages d'une source : son menu remplace le menu principal (« ‹ » ramène aux Intégrations), comme le fork.
export const Route = createFileRoute("/t/$team/integrations/sources/$source")({
  staticData: {
    nav: (p) => {
      const at = (path: string) => `/t/${p.team}/integrations/sources/${p.source}${path}`
      return {
        label: "Source",
        groups: [
          {
            items: [
              { label: "Général", icon: SettingsIcon, to: at(""), exact: true },
              { label: "Permissions", icon: KeyRoundIcon, to: at("/permissions") },
              { label: "Ressources", icon: LayoutGridIcon, to: at("/resources") },
            ],
          },
          { items: [{ label: "Zone de danger", icon: ShieldAlertIcon, to: at("/danger"), danger: true }] },
        ],
      }
    },
  },
  component: Outlet,
})
