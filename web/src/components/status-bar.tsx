import { CircleAlert, CircleCheck, Loader2 } from "lucide-react"

export function StatusBar({
  problems,
  checking,
  status,
}: {
  problems: string[]
  checking?: boolean

  status?: string
}) {
  const failed = problems.length > 0

  return (
    <footer
      className="shrink-0 border-t"
      role="status"
      aria-live="polite"
      aria-atomic="true"
    >
      {failed ? (
        <ul className="max-h-28 overflow-y-auto px-3 py-1.5">
          {problems.map((problem) => (
            <li
              key={problem}
              className="flex items-start gap-1.5 py-0.5 text-xs text-destructive"
            >
              <CircleAlert className="mt-0.5 size-3.5 shrink-0" />
              <span>{problem}</span>
            </li>
          ))}
        </ul>
      ) : (
        <div className="flex h-7 items-center gap-1.5 px-3 text-xs">
          {checking ? (
            <>
              <Loader2 className="size-3.5 animate-spin text-muted-foreground" />
              <span className="text-muted-foreground">Checking the request…</span>
            </>
          ) : (
            <>
              <CircleCheck className="size-3.5 text-emerald-500" />
              <span className="text-muted-foreground">
                {status ?? "Request is valid for this provider."}
              </span>
            </>
          )}
        </div>
      )}
    </footer>
  )
}
