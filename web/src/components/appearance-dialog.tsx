import { useState, type ChangeEvent } from "react"
import { CheckIcon, ImageIcon, MonitorIcon, MoonIcon, SunIcon, UploadIcon, XIcon } from "lucide-react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useTheme, WALLPAPERS, type Theme } from "@/lib/theme"
import { cn } from "@/lib/utils"

export function AppearanceDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
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
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md p-6 sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-xl font-semibold tracking-tight">Apparence & Fond d'écran</DialogTitle>
          <DialogDescription>Personnalisez l'affichage Apple Design, les effets de verre et les fonds d'écran.</DialogDescription>
        </DialogHeader>

        <div className="space-y-6 pt-2">
          {/* Mode clair / sombre / système (Segmented control macOS) */}
          <div className="space-y-2">
            <label className="text-xs font-semibold tracking-wider text-muted-foreground uppercase">Thème</label>
            <div className="flex rounded-xl border border-white/10 bg-black/10 p-1 backdrop-blur-md dark:bg-white/5">
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
                      "flex flex-1 items-center justify-center gap-2 rounded-lg py-1.5 text-xs font-medium transition-all",
                      active
                        ? "bg-card text-foreground shadow-[inset_0_1px_0_0_rgba(255,255,255,0.2),0_2px_8px_rgba(0,0,0,0.15)]"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    <Icon className="size-3.5" />
                    {item.label}
                  </button>
                )
              })}
            </div>
          </div>

          {/* Grille des fonds d'écran */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="text-xs font-semibold tracking-wider text-muted-foreground uppercase">Fond d'écran</label>
              <span className="text-xs text-muted-foreground">Inspiré de macOS</span>
            </div>
            <div className="grid grid-cols-4 gap-3 sm:grid-cols-5">
              {WALLPAPERS.map((wp) => {
                const active = wallpaper === wp.id
                const isPhoto = wp.kind === "photo"
                const isGradient = wp.kind === "gradient"
                const isCustom = wp.kind === "custom"

                return (
                  <button
                    key={wp.id}
                    onClick={() => setWallpaper(wp.id)}
                    className="group flex flex-col items-center gap-1.5 text-center focus:outline-none"
                  >
                    <div
                      className={cn(
                        "relative size-14 shrink-0 rounded-xl border transition-all overflow-hidden shadow-xs",
                        active
                          ? "border-primary ring-2 ring-primary/40 ring-offset-2 ring-offset-background scale-105"
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
                            <ImageIcon className="size-5 opacity-60" />
                          )}
                        </div>
                      )}
                      {active && (
                        <div className="absolute inset-0 flex items-center justify-center bg-black/30 backdrop-blur-[2px]">
                          <CheckIcon className="size-4 text-white drop-shadow" />
                        </div>
                      )}
                    </div>
                    <span className={cn("text-[11px] truncate max-w-full font-medium", active ? "text-primary font-semibold" : "text-muted-foreground")}>
                      {wp.name}
                    </span>
                  </button>
                )
              })}
            </div>
          </div>

          {/* Image personnalisée si sélectionnée */}
          {wallpaper === "custom" && (
            <div className="space-y-2 rounded-xl border border-white/10 bg-white/5 p-3 backdrop-blur-md">
              <label className="text-xs font-medium text-foreground">Téléverser votre propre image</label>
              <div className="flex gap-2">
                <input
                  type="text"
                  placeholder="https://images.unsplash.com/..."
                  value={customUrl}
                  onChange={(e) => setCustomUrl(e.target.value)}
                  className="flex-1 rounded-lg border border-input/60 bg-card/60 px-2.5 py-1 text-xs outline-none focus:border-primary"
                />
                <button
                  type="button"
                  onClick={handleApplyUrl}
                  disabled={!customUrl.trim()}
                  className="rounded-lg bg-primary px-3 py-1 text-xs font-medium text-white transition-all disabled:opacity-50"
                >
                  OK
                </button>
              </div>
              <div className="flex items-center gap-2 pt-1">
                <label className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-white/10 bg-card/40 px-3 py-1.5 text-xs text-muted-foreground hover:bg-card hover:text-foreground">
                  <UploadIcon className="size-3.5" />
                  <span>Choisir un fichier</span>
                  <input type="file" accept="image/*" className="hidden" onChange={handleFileUpload} />
                </label>
                {customImage && (
                  <button
                    type="button"
                    onClick={() => {
                      setCustomImage(null)
                      setWallpaper("forest")
                    }}
                    className="flex items-center gap-1 text-xs text-destructive hover:underline"
                  >
                    <XIcon className="size-3" />
                    Effacer
                  </button>
                )}
              </div>
            </div>
          )}

          {/* Curseur de transparence / intensité du verre */}
          <div className="space-y-2 pt-2 border-t border-white/10">
            <div className="flex items-center justify-between text-xs">
              <span className="font-semibold tracking-wider text-muted-foreground uppercase">Effet de verre (Flou)</span>
              <span className="font-mono text-muted-foreground">{glassIntensity}%</span>
            </div>
            <input
              type="range"
              min={0}
              max={100}
              value={glassIntensity}
              onChange={(e) => setGlassIntensity(Number(e.target.value))}
              className="w-full accent-primary cursor-pointer h-1.5 rounded-lg bg-white/15"
            />
            <div className="flex justify-between text-[10px] text-muted-foreground">
              <span>Subtil</span>
              <span>Intense</span>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
