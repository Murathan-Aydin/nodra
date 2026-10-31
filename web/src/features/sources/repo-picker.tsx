import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { LockIcon } from "lucide-react"
import { Field } from "@/components/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import { branchesQuery, reposQuery } from "./queries"

/** Choix d'un dépôt puis d'une branche d'une source Git (F7-03). */
export function RepoPicker({
  team,
  source,
  repo,
  branch,
  onChange,
}: {
  team: string
  source: string
  repo: string
  branch: string
  onChange: (v: { repo: string; branch: string }) => void
}) {
  const repos = useQuery(reposQuery(team, source))
  const branches = useQuery(branchesQuery(team, source, repo))
  const [filter, setFilter] = useState("")
  const shown = repos.data?.filter((r) => !filter || r.full_name.toLowerCase().includes(filter.toLowerCase())).slice(0, 100)
  return (
    <div className="space-y-3">
      <Field label="Dépôt" error={repos.error instanceof ApiError ? repos.error.message : undefined}>
        <div className="space-y-2">
          {(repos.data?.length ?? 0) > 8 && <Input placeholder="Rechercher un dépôt" value={filter} onChange={(e) => setFilter(e.target.value)} />}
          <Select
            value={repo}
            onValueChange={(v) => {
              const r = repos.data?.find((x) => x.full_name === v)
              onChange({ repo: v, branch: r?.default_branch ?? "main" })
            }}
          >
            <SelectTrigger>
              <SelectValue placeholder={repos.isPending ? "Chargement des dépôts…" : "Choisir un dépôt"} />
            </SelectTrigger>
            <SelectContent>
              {shown?.map((r) => (
                <SelectItem key={r.full_name} value={r.full_name}>
                  <span className="flex items-center gap-1.5">
                    {r.private && <LockIcon className="size-3 text-muted-foreground" />}
                    {r.full_name}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </Field>
      {repo && (
        <Field label="Branche">
          <Select value={branch} onValueChange={(b) => onChange({ repo, branch: b })}>
            <SelectTrigger>
              <SelectValue placeholder={branches.isPending ? "Chargement…" : "Branche"} />
            </SelectTrigger>
            <SelectContent>
              {(branches.data ?? [branch]).map((b) => (
                <SelectItem key={b} value={b}>
                  {b}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}
    </div>
  )
}
