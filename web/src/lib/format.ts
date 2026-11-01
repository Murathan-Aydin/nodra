// Formats d'affichage partagés (dates, durées, tailles) et petites actions navigateur.

const rtf = new Intl.RelativeTimeFormat("fr", { numeric: "auto" })
const dtf = new Intl.DateTimeFormat("fr", { dateStyle: "medium", timeStyle: "short" })

/** « il y a 3 min », « dans 2 h »… */
export function relativeTime(date: string | Date | null | undefined): string {
  if (!date) return "—"
  const s = (new Date(date).getTime() - Date.now()) / 1000
  const abs = Math.abs(s)
  if (abs < 45) return "à l'instant"
  if (abs < 3600) return rtf.format(Math.round(s / 60), "minute")
  if (abs < 86400) return rtf.format(Math.round(s / 3600), "hour")
  if (abs < 86400 * 30) return rtf.format(Math.round(s / 86400), "day")
  if (abs < 86400 * 365) return rtf.format(Math.round(s / (86400 * 30)), "month")
  return rtf.format(Math.round(s / (86400 * 365)), "year")
}

/** « 1 nov. 2026, 14:05 ». */
export function dateTime(date: string | Date | null | undefined): string {
  return date ? dtf.format(new Date(date)) : "—"
}

/** Durée entre deux dates (ou jusqu'à maintenant) : « 42 s », « 3 min 05 s », « 1 h 12 min ». */
export function duration(from: string | Date | null | undefined, to?: string | Date | null): string {
  if (!from) return "—"
  const s = Math.max(0, Math.round(((to ? new Date(to) : new Date()).getTime() - new Date(from).getTime()) / 1000))
  if (s < 60) return `${s} s`
  if (s < 3600) return `${Math.floor(s / 60)} min ${String(s % 60).padStart(2, "0")} s`
  return `${Math.floor(s / 3600)} h ${String(Math.floor((s % 3600) / 60)).padStart(2, "0")} min`
}

/** Taille lisible en unités binaires : « 512 o », « 1,5 Kio », « 2,3 Gio ». */
export function bytes(n: number | null | undefined): string {
  if (n == null) return "—"
  const units = ["o", "Kio", "Mio", "Gio", "Tio"]
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toLocaleString("fr", { maximumFractionDigits: i ? 1 : 0 })} ${units[i]}`
}

/** Copie dans le presse-papiers ; renvoie vrai si la copie a réussi. */
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

/** Télécharge un texte en fichier. */
export function downloadText(name: string, text: string, type = "text/plain") {
  const url = URL.createObjectURL(new Blob([text], { type }))
  downloadUrl(url, name)
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Télécharge une URL (lien présigné, route de l'API…). */
export function downloadUrl(url: string, name = "") {
  const a = document.createElement("a")
  a.href = url
  a.download = name
  document.body.append(a)
  a.click()
  a.remove()
}
