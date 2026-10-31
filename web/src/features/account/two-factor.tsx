import { useQuery } from "@tanstack/react-query"
import QRCode from "qrcode"
import { useEffect, useState } from "react"
import { DownloadIcon, ShieldCheckIcon, ShieldOffIcon } from "lucide-react"
import { CopyButton } from "@/components/copy-button"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { meQuery } from "@/lib/queries"

/** Double authentification TOTP : QR code, activation par code, codes de secours, désactivation (F2-02). */
export function TwoFactorSection() {
  const me = useQuery(meQuery)
  const enabled = !!me.data?.two_factor_enabled
  const [setup, setSetup] = useState<{ secret: string; otpauth_url: string } | null>(null)
  const [code, setCode] = useState("")
  const [codes, setCodes] = useState<string[] | null>(null)
  const [disabling, setDisabling] = useState(false)
  const [password, setPassword] = useState("")
  const start = useApiMutation(() => post<{ secret: string; otpauth_url: string }>("/me/2fa/setup"), { onSuccess: setSetup })
  const enable = useApiMutation((c: string) => post<{ recovery_codes: string[] }>("/me/2fa/enable", { code: c }), {
    success: "Double authentification activée",
    invalidate: [["me"]],
    onSuccess: (r) => {
      setSetup(null)
      setCode("")
      setCodes(r.recovery_codes)
    },
  })
  const disable = useApiMutation((p: string) => post("/me/2fa/disable", { password: p }), {
    success: "Double authentification désactivée",
    invalidate: [["me"]],
    onSuccess: () => {
      setDisabling(false)
      setPassword("")
    },
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          Double authentification
          {enabled ? <Badge variant="success">Activée</Badge> : <Badge variant="outline">Désactivée</Badge>}
        </CardTitle>
        <CardDescription>Code à 6 chiffres d'une application (Authy, 1Password, Google Authenticator…) demandé à chaque connexion.</CardDescription>
      </CardHeader>
      <CardContent>
        {enabled ? (
          <Button variant="outline" onClick={() => setDisabling(true)}>
            <ShieldOffIcon /> Désactiver
          </Button>
        ) : setup ? (
          <form
            className="grid gap-6 md:grid-cols-[auto_1fr]"
            onSubmit={(e) => {
              e.preventDefault()
              enable.mutate(code.replace(/\s/g, ""))
            }}
          >
            <QR value={setup.otpauth_url} />
            <div className="space-y-4">
              <p className="text-sm">1. Scannez le QR code avec votre application, ou saisissez la clé :</p>
              <div className="flex items-center gap-2 rounded-md border px-3 py-1.5">
                <code className="flex-1 font-mono text-sm break-all">{setup.secret}</code>
                <CopyButton value={setup.secret} />
              </div>
              <Field label="2. Code affiché par l'application">
                <Input inputMode="numeric" autoComplete="one-time-code" className="w-40 font-mono tracking-widest" value={code} onChange={(e) => setCode(e.target.value)} autoFocus />
              </Field>
              <div className="flex gap-2">
                <Button type="submit" loading={enable.isPending} disabled={code.replace(/\s/g, "").length < 6}>
                  Activer
                </Button>
                <Button type="button" variant="ghost" onClick={() => setSetup(null)}>
                  Annuler
                </Button>
              </div>
            </div>
          </form>
        ) : (
          <Button onClick={() => start.mutate(undefined)} loading={start.isPending}>
            <ShieldCheckIcon /> Configurer
          </Button>
        )}
      </CardContent>

      <Dialog open={!!codes} onOpenChange={(o) => !o && setCodes(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Codes de secours</DialogTitle>
            <DialogDescription>Affichés une seule fois. Chaque code remplace une fois le code de l'application si vous la perdez.</DialogDescription>
          </DialogHeader>
          <div className="grid grid-cols-2 gap-2 rounded-lg bg-muted p-4 font-mono text-sm">
            {codes?.map((c) => (
              <span key={c}>{c}</span>
            ))}
          </div>
          <DialogFooter>
            <CopyButton value={codes?.join("\n") ?? ""} label="Copier les codes" />
            <Button variant="outline" onClick={() => codes && downloadText("nodra-codes-de-secours.txt", codes.join("\n") + "\n")}>
              <DownloadIcon /> Télécharger
            </Button>
            <Button onClick={() => setCodes(null)}>J'ai conservé mes codes</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={disabling} onOpenChange={setDisabling}>
        <DialogContent>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              disable.mutate(password)
            }}
          >
            <DialogHeader>
              <DialogTitle>Désactiver la double authentification ?</DialogTitle>
              <DialogDescription>Votre compte ne sera plus protégé que par son mot de passe.</DialogDescription>
            </DialogHeader>
            <Field label="Mot de passe" className="my-4">
              <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" autoFocus />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDisabling(false)}>
                Annuler
              </Button>
              <Button type="submit" variant="destructive" loading={disable.isPending} disabled={!password}>
                Désactiver
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

function QR({ value }: { value: string }) {
  const [src, setSrc] = useState("")
  useEffect(() => {
    let alive = true
    void QRCode.toDataURL(value, { margin: 1, width: 200 }).then((url) => alive && setSrc(url))
    return () => {
      alive = false
    }
  }, [value])
  return src ? <img src={src} alt="QR code de configuration" className="size-48 rounded-lg border bg-white p-1" /> : <div className="size-48 rounded-lg border" />
}

export function downloadText(filename: string, content: string) {
  const url = URL.createObjectURL(new Blob([content], { type: "text/plain" }))
  const a = document.createElement("a")
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}
