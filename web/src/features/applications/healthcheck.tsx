import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { SwitchRow } from "@/features/resources/operations"
import type { ResourceRef } from "@/features/resources/resource"
import { useDraft, useResourcePatch } from "@/features/resources/use-patch"
import type { Application, HealthCheck } from "@/lib/types"
import { SaveButton } from "./general"

/** Onglet « Healthcheck » : vérification de santé attendue avant de basculer vers la nouvelle version. */
export function ApplicationHealthcheck({ resource, app }: { resource: ResourceRef; app: Application }) {
  const h = app.health_check
  const { draft: d, set, changed, dirty } = useDraft({ ...h })
  const save = useResourcePatch(resource, "Healthcheck enregistré (appliqué au prochain déploiement)")
  const text = (k: keyof HealthCheck, label: string, hint?: string) => (
    <Field label={label} hint={hint}>
      <Input value={String(d[k] ?? "")} onChange={(e) => set(k, (typeof h[k] === "number" ? Number(e.target.value.replace(/\D/g, "")) : e.target.value) as never)} className="font-mono" />
    </Field>
  )
  return (
    <SettingsCard title="Healthcheck" description="Sans vérification, la bascule se fait dès que le conteneur démarre." actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ health_check: changed })} />}>
      <SwitchRow label="Activé" checked={d.enabled} onChange={(v) => set("enabled", v)} />
      {d.enabled && (
        <div className="mt-2 grid gap-4 sm:grid-cols-3">
          <Field label="Type">
            <Select value={d.type} onValueChange={(v) => set("type", v as HealthCheck["type"])}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="http">Requête HTTP</SelectItem>
                <SelectItem value="cmd">Commande</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          {d.type === "http" ? (
            <>
              {text("scheme", "Schéma")}
              {text("method", "Méthode")}
              {text("host", "Hôte")}
              {text("port", "Port", "Vide = premier port exposé.")}
              {text("path", "Chemin")}
              {text("return_code", "Code attendu")}
              {text("response_text", "Texte attendu", "Facultatif.")}
            </>
          ) : (
            <div className="sm:col-span-2">{text("command", "Commande")}</div>
          )}
          {text("interval", "Intervalle (s)")}
          {text("timeout", "Délai (s)")}
          {text("retries", "Essais")}
          {text("start_period", "Délai de démarrage (s)")}
        </div>
      )}
    </SettingsCard>
  )
}
