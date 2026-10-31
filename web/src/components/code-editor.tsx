import CodeMirror from "@uiw/react-codemirror"
import { yaml } from "@codemirror/lang-yaml"
import { json } from "@codemirror/lang-json"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

/** Éditeur de code (YAML, JSON ou texte), aux couleurs du thème. */
export function CodeEditor({
  value,
  onChange,
  language = "text",
  minHeight = "240px",
  readOnly,
  className,
}: {
  value: string
  onChange?: (v: string) => void
  language?: "yaml" | "json" | "text"
  minHeight?: string
  readOnly?: boolean
  className?: string
}) {
  const { resolved } = useTheme()
  const extensions = language === "yaml" ? [yaml()] : language === "json" ? [json()] : []
  return (
    <div className={cn("overflow-hidden rounded-md border text-sm", className)}>
      <CodeMirror
        value={value}
        onChange={onChange}
        extensions={extensions}
        theme={resolved}
        readOnly={readOnly}
        minHeight={minHeight}
        maxHeight="70vh"
        basicSetup={{ foldGutter: false, highlightActiveLine: !readOnly }}
      />
    </div>
  )
}
