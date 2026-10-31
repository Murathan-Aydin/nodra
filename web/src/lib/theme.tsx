import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react"

export type Theme = "light" | "dark" | "system"

interface ThemeState {
  theme: Theme
  resolved: "light" | "dark"
  setTheme: (t: Theme) => void
}

const ThemeContext = createContext<ThemeState | null>(null)
const KEY = "nodra-theme"

function readTheme(): Theme {
  try {
    const t = localStorage.getItem(KEY)
    if (t === "light" || t === "dark" || t === "system") return t
  } catch {
    // stockage indisponible (navigation privée) : préférence système
  }
  return "system"
}

const systemDark = () => matchMedia("(prefers-color-scheme: dark)").matches

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readTheme)
  const [sysDark, setSysDark] = useState(systemDark)

  useEffect(() => {
    const mq = matchMedia("(prefers-color-scheme: dark)")
    const onChange = () => setSysDark(mq.matches)
    mq.addEventListener("change", onChange)
    return () => mq.removeEventListener("change", onChange)
  }, [])

  const resolved = theme === "system" ? (sysDark ? "dark" : "light") : theme
  useEffect(() => {
    document.documentElement.classList.toggle("dark", resolved === "dark")
  }, [resolved])

  const value = useMemo<ThemeState>(
    () => ({
      theme,
      resolved,
      setTheme: (t) => {
        setThemeState(t)
        try {
          localStorage.setItem(KEY, t)
        } catch {
          // ignoré
        }
      },
    }),
    [theme, resolved],
  )
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeState {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error("ThemeProvider manquant")
  return ctx
}
