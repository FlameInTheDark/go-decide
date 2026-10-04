import { PanelLeftClose, PanelLeftOpen, Play, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { TemplateMenu } from "@/components/template-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Template } from "@/lib/templates"

interface RunBarProps {
  running: boolean
  valid: boolean
  problemCount: number
  onRun: () => void
  onClear: () => void
  onTemplate: (template: Template) => void
  template: string | null
  onToggleHistory: () => void
  historyOpen: boolean
  model?: string
  provider?: string
}

export function RunBar({
  running,
  valid,
  problemCount,
  onRun,
  onClear,
  onTemplate,
  template,
  onToggleHistory,
  historyOpen,
  model,
  provider,
}: RunBarProps) {
  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b bg-background px-4">
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate font-medium">decide</span>
        <span className="text-muted-foreground">playground</span>
      </div>

      <Separator orientation="vertical" />

      <div className="flex min-w-0 items-center gap-1.5 text-sm text-muted-foreground">
        <span className="truncate">{provider || "ollama"}</span>
        {model ? (
          <>
            <span aria-hidden>·</span>
            <span className="truncate font-mono text-xs">{model}</span>
          </>
        ) : null}
      </div>

      <div className="ml-auto flex items-center gap-2">
        {problemCount > 0 ? (
          <span className="text-xs text-destructive">
            {problemCount} {problemCount === 1 ? "problem" : "problems"}
          </span>
        ) : null}

        <TemplateMenu current={template} onPick={onTemplate} />

        <Button variant="ghost" size="sm" onClick={onClear} disabled={running}>
          <Trash2 />
          Clear
        </Button>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={onToggleHistory}
                aria-label={historyOpen ? "Hide history" : "Show history"}
              />
            }
          >
            {historyOpen ? <PanelLeftClose /> : <PanelLeftOpen />}
          </TooltipTrigger>
          <TooltipContent>{historyOpen ? "Hide history" : "Show history"}</TooltipContent>
        </Tooltip>

        <Button size="sm" onClick={onRun} disabled={running || !valid}>
          <Play />
          {running ? "Running" : "Run"}
        </Button>
      </div>
    </header>
  )
}
