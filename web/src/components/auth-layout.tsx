import { useQuery } from "@tanstack/react-query"
import type { ReactNode } from "react"
import { instanceQuery } from "@/lib/queries"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Logo } from "@/components/logo"

/** Mise en page des écrans publics (connexion, inscription…). */
export function AuthLayout({ title, description, children, footer }: { title: string; description?: ReactNode; children: ReactNode; footer?: ReactNode }) {
  const { data: instance } = useQuery(instanceQuery)
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-gradient-to-b from-accent/60 to-background px-4 py-10">
      <div className="mb-6 flex items-center gap-2.5">
        <Logo className="size-9" />
        <span className="text-xl font-semibold tracking-tight">{instance?.name ?? "Nodra"}</span>
      </div>
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-xl">{title}</CardTitle>
          {description && <CardDescription>{description}</CardDescription>}
        </CardHeader>
        <CardContent>{children}</CardContent>
      </Card>
      {footer && <div className="mt-5 text-center text-sm text-muted-foreground">{footer}</div>}
      {instance?.version && <p className="mt-8 text-xs text-muted-foreground">Nodra {instance.version}</p>}
    </div>
  )
}
