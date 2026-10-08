import { useEffect, useState } from "react"
import { Check, ChevronDown, Copy, Terminal } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn } from "cn"

export function CodeBlock({
  value,
  label,
  hint,
  className,
  maxHeight = "16rem",
  collapsible = false,
}: {
  value: unknown
  label?: string
  /** A short explanation shown next to the label while collapsed. */
  hint?: string
  className?: string
  maxHeight?: string
  /**
   * Hides the payload behind a toggle. A raw provider response is large and
   * rarely what you are looking for, so the collapsed header is the cheaper
   * default and the payload stays one click away.
   */
  collapsible?: boolean
}) {
  const [copied, setCopied] = useState(false)
  const [open, setOpen] = useState(false)

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
      <div className={cn("flex h-8 shrink-0 items-center gap-2 px-2", open && "border-b")}>
        {collapsible ? (
          <button
            type="button"
            onClick={() => setOpen(!open)}
            aria-expanded={open}
            className="flex min-w-0 flex-1 items-center gap-2 text-left transition-colors hover:text-foreground"
          >
            <ChevronDown
              className={cn(
                "size-3.5 shrink-0 text-muted-foreground transition-transform",
                !open && "-rotate-90",
              )}
            />
            <span className="truncate text-xs text-muted-foreground">{label}</span>
            {!open && hint ? (
              <span className="truncate text-[0.7rem] text-muted-foreground/70">{hint}</span>
            ) : null}
          </button>
        ) : (
          <>
            <Terminal className="size-3.5 shrink-0 text-muted-foreground" />
            <span className="truncate text-xs text-muted-foreground">{label}</span>
          </>
        )}
        <Button
          variant="ghost"
          size="icon-xs"
          className="shrink-0"
          onClick={copy}
          aria-label={`Copy ${label ?? "JSON"}`}
        >
          {copied ? <Check /> : <Copy />}
        </Button>
      </div>
      {!collapsible || open ? (
        <pre
          className="min-h-0 flex-1 overflow-auto p-3 font-mono text-xs leading-relaxed"
          style={{ maxHeight }}
        >
          {text}
        </pre>
      ) : null}
    </div>
  )
}