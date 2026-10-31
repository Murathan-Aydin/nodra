// Formats d'affichage (français).

const rtf = new Intl.RelativeTimeFormat("fr", { numeric: "auto" })
const dtf = new Intl.DateTimeFormat("fr-FR", { dateStyle: "medium", timeStyle: "short" })

/** « il y a 3 minutes », « dans 2 jours ». */
export function relativeTime(date: string | Date | null | undefined, now = Date.now()): string {
  if (!date) return "—"
  const d = typeof date === "string" ? new Date(date) : date
  const diff = (d.getTime() - now) / 1000
  const abs = Math.abs(diff)
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ["year", 31536000],
    ["month", 2592000],
    ["week", 604800],
    ["day", 86400],
    ["hour", 3600],
    ["minute", 60],
  ]
  for (const [unit, sec] of units) {
    if (abs >= sec) return rtf.format(Math.round(diff / sec), unit)
  }
  return abs < 10 ? "à l'instant" : rtf.format(Math.round(diff), "second")
}

export function dateTime(date: string | Date | null | undefined): string {
  if (!date) return "—"
  return dtf.format(typeof date === "string" ? new Date(date) : date)
}

/** Taille en octets → « 12,3 Mio ». */
export function bytes(n: number | null | undefined): string {
  if (n == null) return "—"
  const units = ["o", "Kio", "Mio", "Gio", "Tio"]
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toLocaleString("fr-FR", { maximumFractionDigits: i === 0 ? 0 : 1 })} ${units[i]}`
}

/** Durée entre deux dates → « 1 min 12 s ». */
export function duration(start: string | null | undefined, end?: string | null): string {
  if (!start) return "—"
  const ms = (end ? new Date(end).getTime() : Date.now()) - new Date(start).getTime()
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return `${s} s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} min ${s % 60} s`
  return `${Math.floor(m / 60)} h ${m % 60} min`
}
