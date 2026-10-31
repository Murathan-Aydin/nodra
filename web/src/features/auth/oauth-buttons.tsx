import { Button } from "@/components/ui/button"
import { API_BASE } from "@/lib/api"
import type { OAuthProvider } from "@/lib/types"

/** Liens de connexion par fournisseur (navigation : le backend redirige vers le fournisseur). */
export function OAuthButtons({ providers, redirect, link }: { providers: OAuthProvider[]; redirect: string; link?: boolean }) {
  return (
    <div className="grid gap-2">
      {providers.map((p) => {
        const qs = new URLSearchParams({ redirect })
        if (link) qs.set("link", "1")
        return (
          <Button key={p.provider} variant="outline" asChild>
            <a href={`${API_BASE}/auth/oauth/${p.provider}/start?${qs}`}>Continuer avec {p.name}</a>
          </Button>
        )
      })}
    </div>
  )
}
