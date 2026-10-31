import { useRouterState } from "@tanstack/react-router"
import { useEffect, useRef, type ReactNode } from "react"

/** Classes des liens d'onglet (à passer à `Link` avec `activeProps={TAB_ACTIVE}`). */
export const TAB_LINK = "-mb-px border-b-2 border-transparent px-3 py-2 text-sm font-medium whitespace-nowrap text-muted-foreground hover:text-foreground"
export const TAB_ACTIVE = { className: "!border-primary !text-foreground" }

/** Barre d'onglets défilante : l'onglet actif reste visible sur les écrans étroits. */
export function TabNav({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLElement>(null)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  useEffect(() => {
    ref.current?.querySelector<HTMLElement>("[data-status=active]")?.scrollIntoView({ block: "nearest", inline: "nearest" })
  }, [pathname])
  return (
    <nav ref={ref} className="mb-6 flex gap-1 overflow-x-auto border-b [scrollbar-width:none]">
      {children}
    </nav>
  )
}
