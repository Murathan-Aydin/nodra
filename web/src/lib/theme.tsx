import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react"

export type Theme = "light" | "dark" | "system"

export interface WallpaperOption {
  id: string
  name: string
  kind: "photo" | "gradient" | "solid" | "custom"
  thumb: string
  image?: string
}

const unsplash = (id: string, width: number) => `https://images.unsplash.com/${id}?auto=format&fit=crop&w=${width}&q=80`

export const WALLPAPERS: WallpaperOption[] = [
  {
    id: "forest",
    name: "Forêt",
    kind: "photo",
    thumb: unsplash("photo-1426604966848-d7adac402bff", 240),
    image: unsplash("photo-1426604966848-d7adac402bff", 2560),
  },
  {
    id: "dolomites",
    name: "Dolomites",
    kind: "photo",
    thumb: unsplash("photo-1506744038136-46273834b3fb", 240),
    image: unsplash("photo-1506744038136-46273834b3fb", 2560),
  },
  {
    id: "pacific",
    name: "Pacifique",
    kind: "photo",
    thumb: unsplash("photo-1507525428034-b723cf961d3e", 240),
    image: unsplash("photo-1507525428034-b723cf961d3e", 2560),
  },
  {
    id: "horizon",
    name: "Horizon",
    kind: "gradient",
    thumb: "linear-gradient(135deg, #38bdf8, #818cf8, #34d399)",
  },
  {
    id: "sunset",
    name: "Crépuscule",
    kind: "gradient",
    thumb: "linear-gradient(135deg, #f97316, #d946ef, #6366f1)",
  },
  {
    id: "neon",
    name: "Néon",
    kind: "gradient",
    thumb: "linear-gradient(135deg, #a855f7, #ec4899, #6366f1)",
  },
  {
    id: "aurora",
    name: "Aurore",
    kind: "gradient",
    thumb: "linear-gradient(135deg, #10b981, #06b6d4, #1d4ed8)",
  },
  {
    id: "graphite",
    name: "Graphite",
    kind: "gradient",
    thumb: "linear-gradient(135deg, #52525b, #18181b, #09090b)",
  },
  {
    id: "none",
    name: "Uni",
    kind: "solid",
    thumb: "#121216",
  },
  {
    id: "custom",
    name: "Personnalisé",
    kind: "custom",
    thumb: "linear-gradient(135deg, rgba(255,255,255,0.1), rgba(255,255,255,0.05))",
  },
]

interface ThemeState {
  theme: Theme
  resolved: "light" | "dark"
  setTheme: (t: Theme) => void
  wallpaper: string
  setWallpaper: (w: string) => void
  customImage: string | null
  setCustomImage: (img: string | null) => void
  glassIntensity: number
  setGlassIntensity: (intensity: number) => void
}

const ThemeContext = createContext<ThemeState | null>(null)
const THEME_KEY = "nodra-theme"
const WALLPAPER_KEY = "nodra-wallpaper"
const IMAGE_KEY = "nodra-wallpaper-image"
const GLASS_KEY = "nodra-glass-intensity"

function readTheme(): Theme {
  try {
    const t = localStorage.getItem(THEME_KEY)
    if (t === "light" || t === "dark" || t === "system") return t
  } catch {
    // ignoré
  }
  return "system"
}

function readWallpaper(): string {
  try {
    const w = localStorage.getItem(WALLPAPER_KEY)
    if (w && WALLPAPERS.some((item) => item.id === w)) return w
  } catch {
    // ignoré
  }
  return "forest"
}

function readCustomImage(): string | null {
  try {
    return localStorage.getItem(IMAGE_KEY)
  } catch {
    return null
  }
}

function readGlassIntensity(): number {
  try {
    const v = localStorage.getItem(GLASS_KEY)
    if (v !== null) {
      const parsed = parseInt(v, 10)
      if (!isNaN(parsed) && parsed >= 0 && parsed <= 100) return parsed
    }
  } catch {
    // ignoré
  }
  return 50
}

const systemDark = () => matchMedia("(prefers-color-scheme: dark)").matches

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readTheme)
  const [sysDark, setSysDark] = useState(systemDark)
  const [wallpaper, setWallpaperState] = useState<string>(readWallpaper)
  const [customImage, setCustomImageState] = useState<string | null>(readCustomImage)
  const [glassIntensity, setGlassIntensityState] = useState<number>(readGlassIntensity)

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

  // Applique le fond d'écran et la variable CSS de l'image
  useEffect(() => {
    const root = document.documentElement
    root.setAttribute("data-wallpaper", wallpaper)

    if (wallpaper === "custom" && customImage) {
      root.style.setProperty("--wallpaper-image", `url("${customImage.replace(/["\\\n]/g, encodeURIComponent)}")`)
    } else {
      const found = WALLPAPERS.find((w) => w.id === wallpaper)
      if (found?.image) {
        root.style.setProperty("--wallpaper-image", `url("${found.image}")`)
      } else {
        root.style.removeProperty("--wallpaper-image")
      }
    }
  }, [wallpaper, customImage])

  // Ajuste l'intensité du verre
  useEffect(() => {
    const root = document.documentElement
    // Échelle : 0 -> très opaque (0 blur, 1 opacity), 100 -> très translucide (36px blur)
    const blurPx = Math.round(12 + (glassIntensity / 100) * 24)
    root.style.setProperty("--glass-blur", `${blurPx}px`)
  }, [glassIntensity])

  const value = useMemo<ThemeState>(
    () => ({
      theme,
      resolved,
      setTheme: (t) => {
        setThemeState(t)
        try {
          localStorage.setItem(THEME_KEY, t)
        } catch {
          // stockage local indisponible (navigation privée) : le réglage reste en mémoire
        }
      },
      wallpaper,
      setWallpaper: (w) => {
        setWallpaperState(w)
        try {
          localStorage.setItem(WALLPAPER_KEY, w)
        } catch {
          // stockage local indisponible (navigation privée) : le réglage reste en mémoire
        }
      },
      customImage,
      setCustomImage: (img) => {
        setCustomImageState(img)
        try {
          if (img) localStorage.setItem(IMAGE_KEY, img)
          else localStorage.removeItem(IMAGE_KEY)
        } catch {
          // stockage local indisponible (navigation privée) : le réglage reste en mémoire
        }
      },
      glassIntensity,
      setGlassIntensity: (intensity) => {
        setGlassIntensityState(intensity)
        try {
          localStorage.setItem(GLASS_KEY, String(intensity))
        } catch {
          // stockage local indisponible (navigation privée) : le réglage reste en mémoire
        }
      },
    }),
    [theme, resolved, wallpaper, customImage, glassIntensity],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeState {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error("ThemeProvider manquant")
  return ctx
}
