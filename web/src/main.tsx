import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { ThemeProvider } from "@/lib/theme"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import "./index.css"

// Front en reconstruction : seul le système visuel (thème, fonds d'écran, composants ui/) est conservé.
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <TooltipProvider delayDuration={300}>
        <Toaster />
      </TooltipProvider>
    </ThemeProvider>
  </StrictMode>,
)
