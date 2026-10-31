import { Toaster as Sonner } from "sonner"
import { useTheme } from "@/lib/theme"

export function Toaster() {
  const { resolved } = useTheme()
  return <Sonner theme={resolved} richColors closeButton position="bottom-right" />
}
