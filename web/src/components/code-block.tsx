import { useEffect, useState } from "react"
import { Check, Copy, Terminal } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn } from "cn"

export function CodeBlock({
  value,
  label,
  className,
  maxHeight = "16rem",
}: {
  value: unknown
  label?: string
  className?: string
  maxHeight?: string
}) {
  const [copied, setCopied] = useState(false)

  const text = typeof value === "string" ? value : JSON.stringify(value, null, 2)

  useEffect(() => {
    if (!copied) return
    const handle = setTimeout(() => setCopied(false), 1500)
    return () => clearTimeout(handle)
  }, [copied])

  const copy = () => {
    navigator.clipboard?.writeText(text).then(
      () => setCopied(true),
      () => undefined,
    )
  }

  return (
    <div className={cn("flex min-h-0 flex-col rounded-lg border bg-muted/40", className)}>
      <div className="flex h-8 shrink-0 items-center gap-2 border-b px-2">
        <Terminal className="size-3.5 text-muted-foreground" />
        <span className="truncate text-xs text-muted-foreground">{label}</span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="ml-auto"
          onClick={copy}
          aria-label="Copy JSON"
        >
          {copied ? <Check /> : <Copy />}
        </Button>
      </div>
      <pre
        className="min-h-0 flex-1 overflow-auto p-3 font-mono text-xs leading-relaxed"
        style={{ maxHeight }}
      >
        {text}
      </pre>
    </div>
  )
}
