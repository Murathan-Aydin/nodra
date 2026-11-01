import CodeMirror from "@uiw/react-codemirror"
import { yaml } from "@codemirror/lang-yaml"
import { json } from "@codemirror/lang-json"
import { useTheme } from "@/lib/theme"

/** Éditeur de code (YAML, JSON ou texte) aux couleurs du thème. */
export function CodeEditor({ value, onChange, language, height = "320px", readOnly }: { value: string; onChange?: (v: string) => void; language?: "yaml" | "json"; height?: string; readOnly?: boolean }) {
  const { resolved } = useTheme()
  return (
    <div className="overflow-hidden rounded-lg border border-border text-xs">
      <CodeMirror
        value={value}
        onChange={onChange}
        height={height}
        readOnly={readOnly}
        theme={resolved === "dark" ? "dark" : "light"}
        extensions={language === "yaml" ? [yaml()] : language === "json" ? [json()] : []}
        basicSetup={{ foldGutter: false, highlightActiveLine: !readOnly }}
      />
    </div>
  )
}
