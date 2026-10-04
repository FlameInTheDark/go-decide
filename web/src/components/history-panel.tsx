import { CircleAlert, CircleCheck, Inbox, RotateCcw } from "lucide-react"

import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { formatDuration } from "@/components/results-panel"
import type { HistoryRun } from "@/lib/types"
import { cn } from "cn"

interface HistoryPanelProps {
  runs: HistoryRun[]

  selected: string | null
  onSelect: (run: HistoryRun) => void
  onRestore: (run: HistoryRun) => void
  onClear: () => void
}

export function HistoryPanel({
  runs,
  selected,
  onSelect,
  onRestore,
  onClear,
}: HistoryPanelProps) {
  return (
    <aside className="flex w-72 shrink-0 flex-col border-l">
      <Header onClear={onClear} count={runs.length} />
      {runs.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
          <Inbox className="size-6 text-muted-foreground" />
          <p className="text-xs text-muted-foreground">
            No runs yet. Every decision is kept here for the last 50 calls, in memory only.
          </p>
        </div>
      ) : (
        <ScrollArea className="flex-1">
          <ul className="space-y-1.5 p-2">
            {runs.map((run) => (
              <li key={run.id}>
                <div
                  className={cn(
                    "group rounded-lg border transition-colors",
                    run.id === selected
                      ? "border-primary/50 bg-primary/10"
                      : "hover:bg-muted/60",
                  )}
                >
                  <button
                    type="button"
                    onClick={() => onSelect(run)}
                    aria-current={run.id === selected ? "true" : undefined}
                    className="w-full p-2 text-left"
                  >
                    <span className="flex items-center gap-1.5 text-[0.7rem] text-muted-foreground">
                      {run.ok ? (
                        <CircleCheck className="size-3.5 text-emerald-500" />
                      ) : (
                        <CircleAlert className="size-3.5 text-destructive" />
                      )}
                      <span className="font-mono">
                        {run.model || run.provider || "request"}
                      </span>
                      <span className="ml-auto tabular-nums">
                        {formatDuration(run.duration_ms)}
                      </span>
                    </span>
                    <span
                      className={cn(
                        "mt-1 block truncate text-xs",
                        run.ok ? "" : "text-destructive",
                      )}
                      title={run.summary || run.error?.error || ""}
                    >
                      {run.summary || run.error?.error || "no summary"}
                    </span>
                    {run.questions && run.questions.length > 0 ? (
                      <span className="mt-0.5 block truncate font-mono text-[0.65rem] text-muted-foreground">
                        {run.questions.join(" · ")}
                      </span>
                    ) : null}
                  </button>
                  <span className="flex items-center justify-between gap-2 px-2 pb-1.5">
                    <span className="truncate font-mono text-[0.6rem] text-muted-foreground/70">
                      {run.at ? new Date(run.at).toLocaleTimeString() : ""}
                    </span>
                    <Button
                      variant="ghost"
                      size="xs"
                      className="h-6 gap-1 px-1.5 text-[0.65rem] opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
                      onClick={() => onRestore(run)}
                      title="Load this request into the editor"
                    >
                      <RotateCcw />
                      Restore
                    </Button>
                  </span>
                </div>
              </li>
            ))}
          </ul>
        </ScrollArea>
      )}
    </aside>
  )
}

function Header({ onClear, count }: { onClear: () => void; count: number }) {
  return (
    <header className="flex h-9 shrink-0 items-center gap-2 border-b px-3">
      <span className="text-xs font-medium">History</span>
      <span className="text-[0.7rem] text-muted-foreground">{count}</span>
      <Button
        variant="ghost"
        size="xs"
        className="ml-auto"
        onClick={onClear}
        disabled={count === 0}
      >
        Clear
      </Button>
    </header>
  )
}
