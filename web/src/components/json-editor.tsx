import { useEffect, useRef } from "react"
import Editor, { loader, useMonaco } from "@monaco-editor/react"
import * as monaco from "monaco-editor/editor/editor.api"
import jsonWorker from "monaco-editor/language/json/json.worker?worker"

import { jsonDefaults } from "monaco-editor/languages/features/json/register"
import { useTheme } from "@/hooks/use-theme"

self.MonacoEnvironment = {
  getWorker() {
    return new jsonWorker()
  },
}

loader.config({ monaco })

jsonDefaults.setDiagnosticsOptions({
  validate: true,
  allowComments: false,
  schemas: [],
})

export function JsonEditor({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  const onChangeRef = useRef(onChange)
  const monaco = useMonaco()
  const theme = useTheme()

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  useEffect(() => {
    monaco?.editor.setTheme(theme === "dark" ? "vs-dark" : "vs")
  }, [monaco, theme])

  return (
    <div className="min-h-0 flex-1 overflow-hidden rounded-md border">
      <Editor
        height="100%"
        language="json"
        value={value}
        theme={theme === "dark" ? "vs-dark" : "vs"}
        onChange={(next) => onChangeRef.current(next ?? "")}
        options={{
          fontSize: 12,
          fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
          lineNumbers: "on",
          lineNumbersMinChars: 3,
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          renderLineHighlight: "none",
          folding: true,
          tabSize: 2,
          automaticLayout: true,
          padding: { top: 10, bottom: 10 },
          scrollbar: { vertical: "auto", horizontal: "auto" },
        }}
      />
    </div>
  )
}
