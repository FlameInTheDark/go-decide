import { ArrowDown, ArrowUp, GripVertical, Plus, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useDragReorder } from "@/hooks/use-drag-reorder"
import { cn } from "cn"

export function ListEditor({
  legend,
  hint,
  rows,
  onChange,
  disabled,
  invalid,
  addLabel,
  placeholder,
}: {
  legend: string
  hint?: string
  rows: string[]
  onChange: (rows: string[]) => void
  disabled?: boolean
  invalid?: boolean
  addLabel: string
  placeholder?: string
}) {
  const { listRef, drag, start, shift } = useDragReorder(rows, onChange, disabled)

  return (
    <fieldset className="space-y-2" disabled={disabled}>
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <Label className="text-xs text-muted-foreground">{legend}</Label>
          {hint ? <p className="mt-0.5 text-[0.7rem] text-muted-foreground">{hint}</p> : null}
        </div>
        <Button
          type="button"
          variant="ghost"
          size="xs"
          onClick={() => onChange([...rows, ""])}
        >
          <Plus />
          {addLabel}
        </Button>
      </div>

      <div ref={listRef} className="space-y-1.5">
        {rows.map((row, index) => (
          <div
            key={index}
            className={cn(
              "flex items-center gap-1.5",
              drag?.to === index && drag.from !== index && "ring-1 ring-primary/60",
            )}
          >
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className="cursor-grab touch-none select-none active:cursor-grabbing"
              aria-label={`Reorder level ${index + 1}`}
              onPointerDown={(event) => start(event, index)}
              disabled={rows.length < 2}
            >
              <GripVertical />
            </Button>

            <span
              className="w-4 shrink-0 text-right font-mono text-[0.65rem] text-muted-foreground"
              aria-hidden
            >
              {index}
            </span>

            <Input
              value={row}
              onChange={(event) => {
                const next = [...rows]
                next[index] = event.target.value
                onChange(next)
              }}
              className={cn("text-xs", invalid && "border-destructive")}
              aria-label={`${legend} ${index + 1}`}
              placeholder={placeholder}
            />

            <div className="flex shrink-0 flex-col">
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                className="h-3.5"
                onClick={() => shift(index, -1)}
                aria-label={`Move level ${index + 1} up`}
                disabled={index === 0}
              >
                <ArrowUp className="size-3" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                className="h-3.5"
                onClick={() => shift(index, 1)}
                aria-label={`Move level ${index + 1} down`}
                disabled={index === rows.length - 1}
              >
                <ArrowDown className="size-3" />
              </Button>
            </div>

            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={() => onChange(rows.filter((_, i) => i !== index))}
              aria-label={`Remove level ${index + 1}`}
              disabled={rows.length <= 1}
            >
              <X />
            </Button>
          </div>
        ))}
      </div>
    </fieldset>
  )
}