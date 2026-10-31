import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** Seuls les chemins internes sont suivis après connexion (pas de redirection ouverte). */
export function safeRedirect(r?: string): string {
  return r && r.startsWith("/") && !r.startsWith("//") && !r.startsWith("/\\") ? r : "/"
}
