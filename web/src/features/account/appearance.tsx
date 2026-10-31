import { useState, type ChangeEvent } from "react"
import { CheckIcon, ImageIcon, MonitorIcon, MoonIcon, SunIcon, UploadIcon, XIcon } from "lucide-react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useTheme, WALLPAPERS, type Theme } from "@/lib/theme"
import { cn } from "@/lib/utils"

export function AppearanceSection() {
  const { theme, setTheme, wallpaper, setWallpaper, customImage, setCustomImage, glassIntensity, setGlassIntensity } = useTheme()
  const [customUrl, setCustomUrl] = useState("")

  const handleFileUpload = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (event) => {
      const result = event.target?.result as string
      if (result) {
        setCustomImage(result)
        setWallpaper("custom")
      }
    }
    reader.readAsDataURL(file)
  }

  const handleApplyUrl = () => {
    if (customUrl.trim()) {
      setCustomImage(customUrl.trim())
      setWallpaper("custom")
      setCustomUrl("")
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Thème & Affichage</CardTitle>
          <CardDescription>Choisissez entre les modes clair, sombre et automatique.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex max-w-sm rounded-xl border border-white/10 bg-black/10 p-1 backdrop-blur-md dark:bg-white/5">
            {(
              [
                { id: "light", label: "Clair", icon: SunIcon },
                { id: "dark", label: "Sombre", icon: MoonIcon },
                { id: "system", label: "Système", icon: MonitorIcon },
              ] as const
            ).map((item) => {
              const Icon = item.icon
              const active = theme === item.id
              return (
                <button
                  key={item.id}
                  onClick={() => setTheme(item.id as Theme)}
                  className={cn(
                    "flex flex-1 items-center justify-center gap-2 rounded-lg py-2 text-xs font-medium transition-all cursor-pointer",
                    active
                      ? "bg-card text-foreground shadow-[inset_0_1px_0_0_rgba(255,255,255,0.2),0_2px_8px_rgba(0,0,0,0.15)] font-semibold"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <Icon className="size-4" />
                  {item.label}
                </button>
              )
            })}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Fond d'écran Apple</CardTitle>
          <CardDescription>Les fonds d'écran se reflètent sous le verre dépoli (liquid glass) de l'interface.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
            {WALLPAPERS.map((wp) => {
              const active = wallpaper === wp.id
              const isPhoto = wp.kind === "photo"
              const isGradient = wp.kind === "gradient"
              const isCustom = wp.kind === "custom"

              return (
                <button
                  key={wp.id}
                  onClick={() => setWallpaper(wp.id)}
                  className="group flex flex-col items-center gap-2 text-center focus:outline-none cursor-pointer"
                >
                  <div
                    className={cn(
                      "relative h-20 w-full rounded-xl border transition-all overflow-hidden shadow-xs",
                      active
                        ? "border-primary ring-2 ring-primary/40 ring-offset-2 ring-offset-background scale-[1.03]"
                        : "border-white/15 hover:border-white/40 hover:scale-[1.02]",
                    )}
                    style={
                      isPhoto
                        ? { backgroundImage: `url(${wp.thumb})`, backgroundSize: "cover", backgroundPosition: "center" }
                        : isGradient
                          ? { background: wp.thumb }
                          : wp.kind === "solid"
                            ? { backgroundColor: wp.thumb }
                            : undefined
                    }
                  >
                    {isCustom && (
                      <div className="flex size-full items-center justify-center bg-white/5 backdrop-blur-xs text-muted-foreground">
                        {customImage ? (
                          <img src={customImage} alt="Personnalisé" className="size-full object-cover" />
                        ) : (
                          <ImageIcon className="size-6 opacity-60" />
                        )}
                      </div>
                    )}
                    {active && (
                      <div className="absolute inset-0 flex items-center justify-center bg-black/30 backdrop-blur-[2px]">
                        <CheckIcon className="size-5 text-white drop-shadow" />
                      </div>
                    )}
                  </div>
                  <span className={cn("text-xs truncate max-w-full font-medium", active ? "text-primary font-semibold" : "text-muted-foreground")}>
                    {wp.name}
                  </span>
                </button>
              )
            })}
          </div>

          {wallpaper === "custom" && (
            <div className="space-y-3 rounded-xl border border-white/10 bg-white/5 p-4 backdrop-blur-md">
              <label className="text-xs font-semibold text-foreground">Image personnalisée</label>
              <div className="flex gap-2">
                <input
                  type="text"
                  placeholder="https://..."
                  value={customUrl}
                  onChange={(e) => setCustomUrl(e.target.value)}
                  className="flex-1 rounded-lg border border-input/60 bg-card/60 px-3 py-1.5 text-xs outline-none focus:border-primary"
                />
                <button
                  type="button"
                  onClick={handleApplyUrl}
                  disabled={!customUrl.trim()}
                  className="rounded-lg bg-primary px-4 py-1.5 text-xs font-medium text-white transition-all disabled:opacity-50"
                >
                  Appliquer
                </button>
              </div>
              <div className="flex items-center gap-3 pt-1">
                <label className="flex cursor-pointer items-center gap-2 rounded-lg border border-white/10 bg-card/40 px-3 py-2 text-xs text-muted-foreground hover:bg-card hover:text-foreground">
                  <UploadIcon className="size-4" />
                  <span>Téléverser depuis l'ordinateur</span>
                  <input type="file" accept="image/*" className="hidden" onChange={handleFileUpload} />
                </label>
                {customImage && (
                  <button
                    type="button"
                    onClick={() => {
                      setCustomImage(null)
                      setWallpaper("forest")
                    }}
                    className="flex items-center gap-1 text-xs text-destructive hover:underline cursor-pointer"
                  >
                    <XIcon className="size-3.5" />
                    Supprimer l'image
                  </button>
                )}
              </div>
            </div>
          )}

          <div className="space-y-3 pt-3 border-t border-white/10 max-w-md">
            <div className="flex items-center justify-between text-xs">
              <span className="font-semibold text-foreground">Intensité de l'effet de verre</span>
              <span className="font-mono text-muted-foreground">{glassIntensity}%</span>
            </div>
            <input
              type="range"
              min={0}
              max={100}
              value={glassIntensity}
              onChange={(e) => setGlassIntensity(Number(e.target.value))}
              className="w-full accent-primary cursor-pointer h-2 rounded-lg bg-white/15"
            />
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
