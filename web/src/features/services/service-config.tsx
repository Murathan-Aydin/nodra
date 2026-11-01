import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { CheckIcon, CopyIcon, EyeIcon, EyeOffIcon } from "lucide-react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { api } from "@/lib/api"
import { serviceQueries } from "@/lib/queries"
import type { EnvVar } from "@/lib/types"

// Variables générées par Nodra (comme Coolify) : SERVICE_<TYPE>_<NOM>.
const GENERATED = /^SERVICE_(USER|PASSWORD(?:WITHSYMBOLS)?(?:_64)?|BASE64(?:_\d+)?|REALBASE64(?:_\d+)?|HEX_\d+|LOWERCASEUSER)_(.+)$/

const KIND: Record<string, string> = { USER: "User", LOWERCASEUSER: "User" }

const word = (w: string) => (w === "MARIADB" ? "MariaDB" : w === "MYSQL" ? "MySQL" : w === "POSTGRES" ? "Postgres" : w.charAt(0) + w.slice(1).toLowerCase())

/** Libellé lisible : SERVICE_PASSWORD_ADMIN → « Admin · Password », MARIADB_DATABASE → « MariaDB Database ». */
export function envLabel(key: string): string {
  const m = GENERATED.exec(key)
  if (!m) return key.split("_").map(word).join(" ")
  const kind = KIND[m[1]] ?? (m[1].startsWith("PASSWORD") ? "Password" : m[1].split("_").map(word).join(" "))
  return `${m[2].split("_").map(word).join(" ")} · ${kind}`
}

const hidden = (v: EnvVar) => v.is_secret || /PASSWORD|SECRET|TOKEN|KEY/.test(v.key)

/** « Configuration du service » : variables du service, modifiables. */
export function ServiceConfig({ team, service }: { team: string; service: string }) {
  const qc = useQueryClient()
  const envs = useQuery(serviceQueries.envs(team, service))
  const [edits, setEdits] = useState<Record<string, string>>({})
  const changed = (envs.data ?? []).filter((v) => edits[v.id] !== undefined && edits[v.id] !== v.value)
  const save = useMutation({
    mutationFn: () => Promise.all(changed.map((v) => api(`/teams/${team}/services/${service}/envs/${v.id}`, { method: "PATCH", json: { value: edits[v.id] } }))),
    onSuccess: () => {
      setEdits({})
      toast.success("Configuration enregistrée")
      void qc.invalidateQueries({ queryKey: ["team", team, "service", service, "envs"] })
    },
    onError: (e) => toast.error(e.message),
  })

  return (
    <SettingsCard
      title="Configuration du service"
      actions={
        <Button size="sm" variant="outline" disabled={changed.length === 0} loading={save.isPending} onClick={() => save.mutate()}>
          Enregistrer
        </Button>
      }
    >
      {envs.isPending ? (
        <Skeleton className="h-32" />
      ) : envs.data?.length ? (
        <div className="grid gap-4 sm:grid-cols-2">
          {envs.data.map((v) => (
            <EnvField key={v.id} env={v} value={edits[v.id] ?? v.value} onChange={(value) => setEdits((e) => ({ ...e, [v.id]: value }))} />
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">Ce service n'a pas de variable.</p>
      )}
    </SettingsCard>
  )
}

function EnvField({ env, value, onChange }: { env: EnvVar; value: string; onChange: (v: string) => void }) {
  const [shown, setShown] = useState(false)
  const [copied, setCopied] = useState(false)
  const secret = hidden(env)
  // Un secret marqué comme tel n'est jamais renvoyé par l'API : rien à afficher ni copier.
  const unreadable = env.is_secret && !env.value
  const copy = () => {
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <Field label={envLabel(env.key)} hint={env.key}>
      <div className="relative">
        <Input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          type={secret && !shown ? "password" : "text"}
          placeholder={unreadable ? "•••••••• (secret)" : undefined}
          className="pr-18 font-mono"
        />
        {!unreadable && (
          <div className="absolute inset-y-0 right-1.5 flex items-center gap-0.5">
            <IconButton label="Copier" onClick={copy}>
              {copied ? <CheckIcon /> : <CopyIcon />}
            </IconButton>
            {secret && (
              <IconButton label={shown ? "Masquer" : "Afficher"} onClick={() => setShown(!shown)}>
                {shown ? <EyeOffIcon /> : <EyeIcon />}
              </IconButton>
            )}
          </div>
        )}
      </div>
    </Field>
  )
}

function IconButton({ label, onClick, children }: { label: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" title={label} aria-label={label} onClick={onClick} className="flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-foreground/5 hover:text-foreground [&_svg]:size-4">
      {children}
    </button>
  )
}
