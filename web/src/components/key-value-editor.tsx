import { Plus, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "cn"

export function KeyValueEditor({
  legend,
  rows,
  onChange,
  disabled,
  invalid,
  addLabel,
  makeRow,
}: {
  legend: string
  rows: { key: string; text: string }[]
  onChange: (rows: { key: string; text: string }[]) => void
  disabled?: boolean
  invalid?: boolean
  addLabel: string
  makeRow: () => { key: string; text: string }
}) {
  return (
    <fieldset className="space-y-2" disabled={disabled}>
      <div className="flex items-center justify-between">
        <Label className="text-xs text-muted-foreground">{legend}</Label>
        <Button
          type="button"
          variant="ghost"
          size="xs"
          onClick={() => onChange([...rows, makeRow()])}
        >
          <Plus />
          {addLabel}
        </Button>
      </div>

      <div className="space-y-1.5">
        {rows.map((row, index) => (
          <div key={index} className="flex items-center gap-1.5">
            <Input
              value={row.key}
              onChange={(event) => {
                const next = [...rows]
                next[index] = { ...row, key: event.target.value }
                onChange(next)
              }}
              className={cn("w-20 shrink-0 font-mono text-xs", invalid && "border-destructive")}
              aria-label={`${legend} key ${index + 1}`}
              spellCheck={false}
            />
            <Input
              value={row.text}
              onChange={(event) => {
                const next = [...rows]
                next[index] = { ...row, text: event.target.value }
                onChange(next)
              }}
              className={cn("text-xs", invalid && "border-destructive")}
              aria-label={`${legend} label ${index + 1}`}
              placeholder="text shown to the model"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={() => onChange(rows.filter((_, i) => i !== index))}
              aria-label={`Remove ${legend} ${index + 1}`}
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
