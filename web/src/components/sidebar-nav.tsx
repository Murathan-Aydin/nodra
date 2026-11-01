import { Link, useMatches, useRouterState } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ChevronLeftIcon } from "lucide-react"
import { NAV, type NavItem, type NavMenu } from "@/components/nav"
import { meQuery } from "@/lib/queries"
import { cn } from "@/lib/utils"

// Les pages des ressources font partie de « Projets » (comme dans Coolify).
const RESOURCE_PAGE = /^\/t\/[^/]+\/(applications|databases|services)\//

/**
 * Navigation de la barre latérale, reprise de navbar.blade.php du fork :
 * - une entrée avec `children` ouvre son sous-menu à la place du menu principal ;
 * - une route qui déclare `staticData.nav` (ressource, serveur…) remplace le menu par le sien ;
 * - « ‹ » revient au menu principal sans quitter la page.
 */
export function SidebarNav({ team, collapsed }: { team: string; collapsed: boolean }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname }).replace(/\/$/, "")
  const matches = useMatches()
  const admin = useQuery(meQuery).data?.user.is_instance_admin ?? false
  const contextMatch = [...matches].reverse().find((m) => m.staticData?.nav)
  const context = contextMatch?.staticData.nav?.(contextMatch.params as Record<string, string>)
  // Choix de l'utilisateur, valables seulement sur la page où il les a faits.
  const [opened, setOpened] = useState<{ path: string; submenu: string } | null>(null)
  const [mainOn, setMainOn] = useState<string | null>(null)

  const base = `/t/${team}`
  const isActive = (path: string) => {
    const href = (base + path).replace(/\/$/, "")
    if (path === "") return pathname === base
    return pathname === href || pathname.startsWith(href + "/") || (path === "/projects" && RESOURCE_PAGE.test(pathname + "/"))
  }
  const items = NAV.flatMap((g) => g.items).filter((i) => !i.admin || admin)
  const activeSub = items.find((i) => i.children?.some((c) => isActive(c.path)))
  const submenuLabel = opened?.path === pathname ? opened.submenu : mainOn === pathname ? "" : activeSub?.label
  const submenu = items.find((i) => i.label === submenuLabel && i.children)

  if (context && mainOn !== pathname) return <ContextMenu menu={context} pathname={pathname} collapsed={collapsed} onBack={() => setMainOn(pathname)} />
  if (submenu) {
    return (
      <SubMenu
        item={submenu}
        base={base}
        isActive={(p) => (p === submenu.path ? pathname === (base + p).replace(/\/$/, "") : isActive(p))}
        collapsed={collapsed}
        onBack={() => {
          setOpened(null)
          setMainOn(pathname)
        }}
      />
    )
  }
  return (
    <>
      {NAV.map((g, i) => {
        const shown = g.items.filter((it) => !it.admin || admin)
        if (shown.length === 0) return null
        return (
          <ul key={i} role="list" className="flex flex-col gap-y-0.5">
            {g.title && !collapsed && (
              <li className="nav-section mt-3" aria-hidden="true">
                {g.title}
              </li>
            )}
            {shown.map((item) => (
              <li key={item.label}>
                {item.children ? (
                  <button
                    type="button"
                    title={collapsed ? item.label : undefined}
                    onClick={() => setOpened({ path: pathname, submenu: item.label })}
                    className={cn("menu-item", item.children.some((c) => isActive(c.path)) && "menu-item-active")}
                  >
                    <item.icon className="menu-item-icon" />
                    {!collapsed && <span className="menu-item-label text-left">{item.label}</span>}
                  </button>
                ) : (
                  <Link to={base + item.path} title={collapsed ? item.label : undefined} className={cn("menu-item", isActive(item.path) && "menu-item-active")}>
                    <item.icon className="menu-item-icon" />
                    {!collapsed && <span className="menu-item-label">{item.label}</span>}
                  </Link>
                )}
              </li>
            ))}
          </ul>
        )
      })}
    </>
  )
}

function BackRow({ label, collapsed, onBack }: { label: string; collapsed: boolean; onBack: () => void }) {
  return (
    <li className="mb-1">
      <button type="button" onClick={onBack} title="Revenir au menu principal" className="menu-item">
        <span className="flex w-[18px] shrink-0 justify-center">
          <ChevronLeftIcon className="size-3.5 text-primary" />
        </span>
        {!collapsed && <span className="flex-1 truncate pr-6 text-center font-medium text-foreground">{label}</span>}
      </button>
    </li>
  )
}

function SubMenu({ item, base, isActive, collapsed, onBack }: { item: NavItem; base: string; isActive: (p: string) => boolean; collapsed: boolean; onBack: () => void }) {
  return (
    <ul role="list" className="flex flex-col gap-y-0.5">
      <BackRow label={item.label} collapsed={collapsed} onBack={onBack} />
      {item.children?.map((c) => (
        <li key={c.path}>
          {c.sectionStart && <div className="my-2 border-t border-border" aria-hidden="true" />}
          <Link to={base + c.path} title={c.label} className={cn("menu-item", isActive(c.path) && "menu-item-active")}>
            <c.icon className={cn("menu-item-icon", c.danger && "text-destructive")} />
            {!collapsed && <span className="menu-item-label">{c.label}</span>}
          </Link>
        </li>
      ))}
    </ul>
  )
}

function ContextMenu({ menu, pathname, collapsed, onBack }: { menu: NavMenu; pathname: string; collapsed: boolean; onBack: () => void }) {
  return (
    <ul role="list" className="flex flex-col gap-y-0.5">
      <BackRow label="Tableau de bord" collapsed={collapsed} onBack={onBack} />
      {menu.groups.map((g, i) => (
        <li key={i}>
          {i > 0 && <div className="my-2 border-t border-border" aria-hidden="true" />}
          {g.title && !collapsed && <p className="nav-section">{g.title}</p>}
          <ul className="flex flex-col gap-y-0.5">
            {g.items.map((it) => {
              const to = it.to.replace(/\/$/, "")
              const active = it.exact ? pathname === to : pathname === to || pathname.startsWith(to + "/")
              return (
                <li key={it.to}>
                  <Link to={it.to} title={it.label} className={cn("menu-item", active && "menu-item-active")}>
                    <it.icon className={cn("menu-item-icon", it.danger && "text-destructive")} />
                    {!collapsed && <span className="menu-item-label">{it.label}</span>}
                  </Link>
                </li>
              )
            })}
          </ul>
        </li>
      ))}
    </ul>
  )
}
