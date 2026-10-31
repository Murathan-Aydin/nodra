import { useQuery } from "@tanstack/react-query"
import { LinkIcon, MonitorIcon, UnlinkIcon } from "lucide-react"
import { useState } from "react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { OAuthButtons } from "@/features/auth/oauth-buttons"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { del, get } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { instanceQuery } from "@/lib/queries"
import type { Identity, UserSession } from "@/lib/types"

/** Navigateur et système lisibles depuis l'User-Agent. */
function describeAgent(ua: string): string {
  const browser = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : /curl\//.test(ua) ? "curl" : "Navigateur"
  const os = /Windows/.test(ua) ? "Windows" : /iPhone|iPad/.test(ua) ? "iOS" : /Mac OS X/.test(ua) ? "macOS" : /Android/.test(ua) ? "Android" : /Linux/.test(ua) ? "Linux" : ""
  return os ? `${browser} sur ${os}` : browser
}

/** Sessions ouvertes, avec révocation (F2-03). */
export function SessionsSection() {
  const sessions = useQuery({ queryKey: ["me", "sessions"], queryFn: () => get<UserSession[]>("/me/sessions") })
  const revoke = useApiMutation((id: string) => del(`/me/sessions/${id}`), { success: "Session fermée", invalidate: [["me", "sessions"]] })
  return (
    <Card>
      <CardHeader>
        <CardTitle>Sessions</CardTitle>
        <CardDescription>Appareils connectés à votre compte. Changer de mot de passe ferme toutes les autres.</CardDescription>
      </CardHeader>
      <CardContent>
        <ul className="divide-y rounded-lg border">
          {sessions.data?.map((s) => (
            <li key={s.id} className="flex items-center gap-3 px-3 py-2.5 text-sm">
              <MonitorIcon className="size-4 text-muted-foreground" />
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-2 font-medium">
                  {describeAgent(s.user_agent)}
                  {s.current && <Badge variant="success">Cette session</Badge>}
                </span>
                <span className="block truncate text-xs text-muted-foreground" title={s.user_agent}>
                  {s.ip} · active {relativeTime(s.last_seen_at)} · ouverte le {dateTime(s.created_at)}
                </span>
              </span>
              {!s.current && (
                <Button variant="outline" size="sm" loading={revoke.isPending && revoke.variables === s.id} onClick={() => revoke.mutate(s.id)}>
                  Fermer
                </Button>
              )}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}

const PROVIDERS: Record<string, string> = { github: "GitHub", gitlab: "GitLab", google: "Google" }

/** Comptes externes liés (connexion OAuth) : liaison et déliaison (F2-05). */
export function IdentitiesSection({ redirect }: { redirect: string }) {
  const instance = useQuery(instanceQuery)
  const identities = useQuery({ queryKey: ["me", "identities"], queryFn: () => get<Identity[]>("/me/identities") })
  const [unlinking, setUnlinking] = useState<Identity | null>(null)
  const unlink = useApiMutation((p: string) => del(`/me/identities/${p}`), {
    success: "Compte délié",
    invalidate: [["me", "identities"]],
    onSuccess: () => setUnlinking(null),
  })
  const linked = new Set(identities.data?.map((i) => i.provider))
  const available = instance.data?.oauth_providers.filter((p) => !linked.has(p.provider)) ?? []
  if (!identities.data?.length && !available.length) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle>Comptes liés</CardTitle>
        <CardDescription>Se connecter avec un compte externe en plus (ou à la place) du mot de passe.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!!identities.data?.length && (
          <ul className="divide-y rounded-lg border">
            {identities.data.map((i) => (
              <li key={i.provider} className="flex items-center gap-3 px-3 py-2.5 text-sm">
                <LinkIcon className="size-4 text-muted-foreground" />
                <span className="min-w-0 flex-1">
                  <span className="block font-medium">{PROVIDERS[i.provider] ?? i.provider}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {i.email || i.subject} · lié {relativeTime(i.created_at)}
                  </span>
                </span>
                <Button variant="outline" size="sm" onClick={() => setUnlinking(i)}>
                  <UnlinkIcon /> Délier
                </Button>
              </li>
            ))}
          </ul>
        )}
        {available.length > 0 && (
          <div className="max-w-sm">
            <OAuthButtons providers={available} redirect={redirect} link />
          </div>
        )}
      </CardContent>
      <ConfirmDialog
        open={!!unlinking}
        onOpenChange={(o) => !o && setUnlinking(null)}
        title={`Délier ${unlinking ? (PROVIDERS[unlinking.provider] ?? unlinking.provider) : ""} ?`}
        description="Vous ne pourrez plus vous connecter avec ce compte. Impossible s'il s'agit de votre seul moyen de connexion."
        confirmLabel="Délier"
        destructive
        loading={unlink.isPending}
        onConfirm={() => unlinking && unlink.mutate(unlinking.provider)}
      />
    </Card>
  )
}
