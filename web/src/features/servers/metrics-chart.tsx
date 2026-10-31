import { useQuery } from "@tanstack/react-query"
import { useMemo, useState } from "react"
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { bytes } from "@/lib/format"
import { metricsQuery } from "./queries"

const RANGES = [
  { label: "1 h", ms: 3600e3 },
  { label: "24 h", ms: 86400e3 },
  { label: "7 j", ms: 7 * 86400e3 },
]

/** CPU, mémoire et disque du serveur (relevés chaque minute). */
export function MetricsCharts({ team, server }: { team: string; server: string }) {
  const [range, setRange] = useState(0)
  const [since] = useState(() => RANGES.map((r) => new Date(Date.now() - r.ms).toISOString()))
  const { data, isPending } = useQuery(metricsQuery(team, server, since[range]))
  const points = useMemo(() => {
    const raw = data ?? []
    // Sous-échantillonnage pour garder ~300 points.
    const step = Math.max(1, Math.ceil(raw.length / 300))
    return raw
      .filter((_, i) => i % step === 0)
      .map((m) => ({
        t: new Date(m.ts).getTime(),
        cpu: Math.round(m.cpu_percent * 10) / 10,
        mem: m.mem_total ? Math.round((m.mem_used / m.mem_total) * 1000) / 10 : 0,
        disk: m.disk_total ? Math.round((m.disk_used / m.disk_total) * 1000) / 10 : 0,
      }))
  }, [data])
  const last = data?.[data.length - 1]
  const fmt = (t: number) =>
    new Date(t).toLocaleString("fr-FR", range === 0 ? { hour: "2-digit", minute: "2-digit" } : { day: "2-digit", month: "2-digit", hour: "2-digit" })

  const series = [
    { key: "cpu", label: "CPU", color: "var(--chart-1)", detail: last ? `${last.cpu_percent.toFixed(1)} % · charge ${last.load1.toFixed(2)}` : "" },
    { key: "mem", label: "Mémoire", color: "var(--chart-2)", detail: last ? `${bytes(last.mem_used)} / ${bytes(last.mem_total)}` : "" },
    { key: "disk", label: "Disque", color: "var(--chart-3)", detail: last ? `${bytes(last.disk_used)} / ${bytes(last.disk_total)}` : "" },
  ] as const

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Métriques</h2>
        <div className="flex gap-1">
          {RANGES.map((r, i) => (
            <Button key={r.label} size="sm" variant={i === range ? "secondary" : "ghost"} onClick={() => setRange(i)}>
              {r.label}
            </Button>
          ))}
        </div>
      </div>
      <div className="grid gap-4 lg:grid-cols-3">
        {series.map((s) => (
          <Card key={s.key}>
            <CardHeader className="pb-1">
              <CardTitle className="flex items-baseline justify-between text-sm">
                {s.label}
                <span className="text-xs font-normal text-muted-foreground">{s.detail}</span>
              </CardTitle>
            </CardHeader>
            <CardContent className="h-36 px-2">
              {isPending ? (
                <Skeleton className="h-full w-full" />
              ) : !points.length ? (
                <p className="pt-10 text-center text-xs text-muted-foreground">Pas encore de relevé.</p>
              ) : (
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={points} margin={{ top: 4, right: 4, bottom: 0, left: -24 }}>
                    <defs>
                      <linearGradient id={`g-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor={s.color} stopOpacity={0.35} />
                        <stop offset="100%" stopColor={s.color} stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid vertical={false} stroke="var(--border)" />
                    <XAxis dataKey="t" type="number" domain={["dataMin", "dataMax"]} tickFormatter={fmt} tick={{ fontSize: 10 }} stroke="var(--muted-foreground)" minTickGap={40} />
                    <YAxis domain={[0, 100]} tick={{ fontSize: 10 }} stroke="var(--muted-foreground)" unit="%" />
                    <Tooltip
                      labelFormatter={(t) => new Date(Number(t)).toLocaleString("fr-FR")}
                      formatter={(v) => [`${v} %`, s.label]}
                      contentStyle={{ background: "var(--popover)", border: "1px solid var(--border)", borderRadius: 8, fontSize: 12 }}
                    />
                    <Area type="monotone" dataKey={s.key} stroke={s.color} fill={`url(#g-${s.key})`} strokeWidth={1.5} isAnimationActive={false} />
                  </AreaChart>
                </ResponsiveContainer>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}
