import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { SettingsCard } from "@/components/page"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Chart, RANGES } from "@/features/servers/tools"
import { get } from "@/lib/api"
import { bytes } from "@/lib/format"
import { resourceApi, resourceKey, type ResourceRef } from "./resource"

type Metric = { ts: string; cpu_percent: number; mem_used: number; mem_limit: number; net_rx: number; net_tx: number; containers: number }

/** Onglet « Métriques » (Metrics du fork) : processeur, mémoire et réseau des conteneurs de la ressource. */
export function ResourceMetricsTab({ resource }: { resource: ResourceRef }) {
  const [range, setRange] = useState("1")
  const metrics = useQuery({
    queryKey: resourceKey(resource, "metrics", range),
    queryFn: () => get<Metric[]>(resourceApi(resource, `/metrics?since=${encodeURIComponent(new Date(Date.now() - Number(range) * 3600_000).toISOString())}`)),
    refetchInterval: 60_000,
  })
  const list = metrics.data ?? []
  // Débit réseau : différence entre deux relevés (les compteurs repartent de zéro au redémarrage des conteneurs).
  const data = list.map((m, i) => {
    const prev = list[i - 1]
    const dt = prev ? (new Date(m.ts).getTime() - new Date(prev.ts).getTime()) / 1000 : 0
    const rate = (a: number, b: number) => (prev && dt > 0 && a >= b ? Math.round(((a - b) / dt / 1024) * 10) / 10 : 0)
    return {
      t: new Date(m.ts).toLocaleTimeString("fr", { hour: "2-digit", minute: "2-digit" }),
      cpu: Math.round(m.cpu_percent * 10) / 10,
      mem: Math.round((m.mem_used / 1024 / 1024) * 10) / 10,
      rx: rate(m.net_rx, prev?.net_rx ?? 0),
      tx: rate(m.net_tx, prev?.net_tx ?? 0),
    }
  })
  const last = list.at(-1)
  return (
    <SettingsCard
      title="Métriques"
      description={
        last
          ? `${last.containers} conteneur(s) · mémoire ${bytes(last.mem_used)}${last.mem_limit ? ` / ${bytes(last.mem_limit)}` : ""} · relevé chaque minute`
          : "Relevées chaque minute avec les métriques du serveur (à activer dans Serveur › Avancé)."
      }
      actions={
        <Select value={range} onValueChange={setRange}>
          <SelectTrigger className="h-8 w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RANGES.map((r) => (
              <SelectItem key={r.value} value={r.value}>
                {r.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      }
    >
      {data.length === 0 ? (
        <p className="text-sm text-muted-foreground">{metrics.isPending ? "Chargement…" : "Aucune mesure sur la période : la ressource ne tourne pas, ou les métriques du serveur sont désactivées."}</p>
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Chart title="Processeur (%, 100 = un cœur)" data={data} k="cpu" color="var(--primary)" />
          <Chart title="Mémoire (Mio)" data={data} k="mem" color="var(--success)" />
          <Chart title="Réseau reçu (Kio/s)" data={data} k="rx" color="var(--warning)" />
          <Chart title="Réseau envoyé (Kio/s)" data={data} k="tx" color="var(--muted-foreground)" />
        </div>
      )}
    </SettingsCard>
  )
}
