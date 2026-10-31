import { Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { patch, post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { meQuery } from "@/lib/queries"

/** Nom, adresse et changement de mot de passe (F2-01). */
export function ProfileSection() {
  const me = useQuery(meQuery)
  const user = me.data?.user
  const [name, setName] = useState<string | null>(null)
  const [pw, setPw] = useState({ current_password: "", new_password: "", confirm: "" })
  const saveName = useApiMutation((n: string) => patch("/me", { name: n }), { success: "Nom enregistré", invalidate: [["me"]], onSuccess: () => setName(null) })
  const resend = useApiMutation(() => post("/me/verify-email"), { success: "Lien de vérification envoyé" })
  const changePw = useApiMutation(() => post("/me/password", { current_password: pw.current_password, new_password: pw.new_password }), {
    success: "Mot de passe changé ; vos autres sessions ont été fermées",
    invalidate: [["me", "sessions"]],
    onSuccess: () => setPw({ current_password: "", new_password: "", confirm: "" }),
  })
  if (!user) return null
  const mismatch = pw.confirm !== "" && pw.confirm !== pw.new_password
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Profil</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              if (name !== null) saveName.mutate(name.trim())
            }}
          >
            <Field label="Nom">
              <Input value={name ?? user.name} onChange={(e) => setName(e.target.value)} autoComplete="name" />
            </Field>
            <Field
              label="Adresse e-mail"
              hint={
                user.email_verified_at ? (
                  <Badge variant="success">Vérifiée</Badge>
                ) : (
                  <span className="flex items-center gap-2">
                    <Badge variant="warning">Non vérifiée</Badge>
                    <Button type="button" variant="link" size="sm" className="h-auto p-0" loading={resend.isPending} onClick={() => resend.mutate(undefined)}>
                      Renvoyer le lien
                    </Button>
                  </span>
                )
              }
            >
              <Input value={user.email} readOnly disabled />
            </Field>
            <div className="flex justify-end">
              <Button type="submit" disabled={name === null || !name.trim() || name.trim() === user.name} loading={saveName.isPending}>
                Enregistrer
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Mot de passe</CardTitle>
          <CardDescription>
            Compte créé avec GitHub, GitLab ou Google sans mot de passe ? Utilisez{" "}
            <Link to="/forgot-password" className="underline">
              mot de passe oublié
            </Link>
            .
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              changePw.mutate(undefined)
            }}
          >
            <Field label="Mot de passe actuel">
              <Input type="password" value={pw.current_password} onChange={(e) => setPw({ ...pw, current_password: e.target.value })} autoComplete="current-password" />
            </Field>
            <Field label="Nouveau mot de passe" hint="8 caractères minimum.">
              <Input type="password" value={pw.new_password} onChange={(e) => setPw({ ...pw, new_password: e.target.value })} autoComplete="new-password" />
            </Field>
            <Field label="Confirmation" error={mismatch ? "Les deux mots de passe diffèrent." : undefined}>
              <Input type="password" value={pw.confirm} onChange={(e) => setPw({ ...pw, confirm: e.target.value })} autoComplete="new-password" />
            </Field>
            <div className="flex justify-end">
              <Button type="submit" disabled={!pw.current_password || pw.new_password.length < 8 || pw.confirm !== pw.new_password} loading={changePw.isPending}>
                Changer le mot de passe
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
