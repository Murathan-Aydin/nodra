import { createFileRoute } from "@tanstack/react-router"
import { useEffect } from "react"
import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/page-header"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { ProfileSection } from "@/features/account/profile"
import { IdentitiesSection, SessionsSection } from "@/features/account/sessions"
import { TokensSection } from "@/features/account/tokens"
import { TwoFactorSection } from "@/features/account/two-factor"

type Tab = "profile" | "security" | "tokens"

export const Route = createFileRoute("/t/$team/account/")({
  validateSearch: (s: Record<string, unknown>): { tab?: Tab; oauth_linked?: string; oauth_error?: string } => ({
    tab: s.tab === "security" || s.tab === "tokens" ? s.tab : undefined,
    oauth_linked: typeof s.oauth_linked === "string" ? s.oauth_linked : undefined,
    oauth_error: typeof s.oauth_error === "string" ? s.oauth_error : undefined,
  }),
  component: Account,
})

function Account() {
  const { team } = Route.useParams()
  const { tab = "profile", oauth_linked, oauth_error } = Route.useSearch()
  const navigate = Route.useNavigate()
  const qc = useQueryClient()
  // Retour de liaison OAuth : message puis nettoyage de l'URL.
  useEffect(() => {
    if (!oauth_linked && !oauth_error) return
    if (oauth_linked) {
      toast.success(`Compte ${oauth_linked} lié`)
      void qc.invalidateQueries({ queryKey: ["me", "identities"] })
    } else toast.error(oauth_error)
    void navigate({ search: { tab: "security" }, replace: true })
  }, [oauth_linked, oauth_error, navigate, qc])
  return (
    <>
      <PageHeader title="Mon compte" description="Profil, sécurité et accès à l'API." />
      <Tabs value={tab} onValueChange={(v) => void navigate({ search: { tab: v as Tab }, replace: true })}>
        <TabsList className="mb-4">
          <TabsTrigger value="profile">Profil</TabsTrigger>
          <TabsTrigger value="security">Sécurité</TabsTrigger>
          <TabsTrigger value="tokens">Jetons API</TabsTrigger>
        </TabsList>
        <TabsContent value="profile">
          <ProfileSection />
        </TabsContent>
        <TabsContent value="security" className="space-y-6">
          <TwoFactorSection />
          <IdentitiesSection redirect={`/t/${team}/account`} />
          <SessionsSection />
        </TabsContent>
        <TabsContent value="tokens">
          <TokensSection team={team} />
        </TabsContent>
      </Tabs>
    </>
  )
}
