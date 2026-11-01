import { useState } from "react"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { ResourceRef } from "./resource"
import { useResourcePatch } from "./use-patch"

const FIELDS: { key: string; label: string; hint: string }[] = [
  { key: "cpus", label: "Processeurs", hint: "« 0 » = illimité, ex. « 1.5 »." },
  { key: "cpuset", label: "Cœurs autorisés", hint: "Ex. « 0-2 » ou « 0,3 »." },
  { key: "cpu_shares", label: "Poids processeur", hint: "1024 par défaut." },
  { key: "memory", label: "Mémoire maximale", hint: "« 0 » = illimitée, ex. « 512m », « 2g »." },
  { key: "memory_swap", label: "Mémoire + swap", hint: "« 0 » = illimitée." },
  { key: "memory_reservation", label: "Mémoire réservée", hint: "Limite souple, « 0 » = aucune." },
  { key: "memory_swappiness", label: "Swappiness", hint: "0 à 100, 60 par défaut." },
]

const NUMERIC = new Set(["cpu_shares", "memory_swappiness"])

/** Carte « Limites » (Resource Limits du fork) : processeur et mémoire du conteneur. */
export function LimitsCard({ resource, limits }: { resource: ResourceRef; limits: Record<string, string | number> }) {
  const [d, setD] = useState(() => Object.fromEntries(FIELDS.map((f) => [f.key, String(limits[f.key] ?? "")])))
  const save = useResourcePatch(resource, "Limites enregistrées (appliquées au prochain démarrage)")
  const dirty = FIELDS.some((f) => d[f.key] !== String(limits[f.key] ?? ""))
  return (
    <SettingsCard
      title="Limites"
      description="Appliquées au prochain démarrage."
      actions={
        <Button
          size="sm"
          variant="outline"
          disabled={!dirty}
          loading={save.isPending}
          onClick={() => save.mutate({ limits: Object.fromEntries(FIELDS.map((f) => [f.key, NUMERIC.has(f.key) ? Number(d[f.key]) : d[f.key]])) })}
        >
          Enregistrer
        </Button>
      }
    >
      <div className="grid gap-4 sm:grid-cols-3">
        {FIELDS.map((f) => (
          <Field key={f.key} label={f.label} hint={f.hint}>
            <Input value={d[f.key]} onChange={(e) => setD({ ...d, [f.key]: e.target.value })} className="font-mono" />
          </Field>
        ))}
      </div>
    </SettingsCard>
  )
}
