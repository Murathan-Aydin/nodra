import { useState } from "react"
import { EyeIcon, EyeOffIcon } from "lucide-react"
import { Input } from "@/components/ui/input"

/** Mot de passe avec bouton afficher / masquer. */
export function PasswordInput(props: Omit<React.ComponentProps<typeof Input>, "type">) {
  const [shown, setShown] = useState(false)
  return (
    <div className="relative">
      <Input {...props} type={shown ? "text" : "password"} className="pr-10" />
      <button
        type="button"
        onClick={() => setShown(!shown)}
        className="absolute inset-y-0 right-0 flex w-10 items-center justify-center text-muted-foreground hover:text-foreground"
        aria-label={shown ? "Masquer le mot de passe" : "Afficher le mot de passe"}
      >
        {shown ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
      </button>
    </div>
  )
}
