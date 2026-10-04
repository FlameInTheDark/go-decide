import { ChevronDown, Copy, GripVertical, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { KeyValueEditor } from "@/components/key-value-editor"
import { ListEditor } from "@/components/list-editor"
import type { DraftQuestion } from "@/lib/draft"
import { nextKey } from "@/lib/draft"
import type { QuestionType } from "@/lib/types"
import { cn } from "cn"

const TYPES: { value: QuestionType; label: string; hint: string }[] = [
  {
    value: "choice",
    label: "Choice",
    hint: "One of several options. Use it to pick a category or a route.",
  },
  {
    value: "noul",
    label: "Yes / no",
    hint: "A binary judgement, such as whether to escalate a message.",
  },
  {
    value: "score",
    label: "Score",
    hint: "A position on an ordered scale, such as urgency from 0 to 3.",
  },
]

interface QuestionEditorProps {
  question: DraftQuestion
  index: number
  total: number
  invalid?: boolean
  problems?: string[]
  expanded: boolean
  onToggle: () => void
  onChange: (question: DraftQuestion) => void
  onMove: (delta: number) => void
  onDuplicate: () => void
  onRemove: () => void
}

export function QuestionEditor({
  question,
  index,
  total,
  invalid,
  problems,
  expanded,
  onToggle,
  onChange,
  onMove,
  onDuplicate,
  onRemove,
}: QuestionEditorProps) {
  const type = TYPES.find((entry) => entry.value === question.type) ?? TYPES[0]
  const patch = (fields: Partial<DraftQuestion>) => onChange({ ...question, ...fields })

  return (
    <div
      className={cn(
        "rounded-lg border bg-card transition-colors",
        invalid && "border-destructive/60",
      )}
    >
      <div className="flex items-center gap-1.5 p-1.5">
        <Button
          variant="ghost"
          size="icon-sm"
          className="cursor-grab"
          aria-label={`Reorder question ${index + 1}`}
          disabled={total < 2}
          onClick={() => onToggle()}
        >
          <GripVertical />
        </Button>

        <button
          type="button"
          onClick={onToggle}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
          aria-expanded={expanded}
        >
          <span
            className={cn(
              "shrink-0 rounded px-1.5 py-0.5 font-mono text-[0.65rem] uppercase",
              "bg-muted text-muted-foreground",
            )}
          >
            {type.label}
          </span>
          <span className="truncate text-sm">
            {question.name || <span className="text-muted-foreground">unnamed question</span>}
          </span>
          <ChevronDown
            className={cn(
              "ml-auto size-4 shrink-0 text-muted-foreground transition-transform",
              !expanded && "-rotate-90",
            )}
          />
        </button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onDuplicate}
          aria-label={`Duplicate question ${index + 1}`}
        >
          <Copy />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onRemove}
          aria-label={`Remove question ${index + 1}`}
          disabled={total < 2}
        >
          <Trash2 />
        </Button>
      </div>

      {expanded ? (
        <div className="space-y-3 border-t p-3">
          <div className="flex gap-2">
            <div className="w-40 shrink-0">
              <label className="mb-1 block text-xs text-muted-foreground" htmlFor={`type-${question.key}`}>
                Type
              </label>
              <Select
                value={question.type}
                onValueChange={(value) => {
                  if (!value) return
                  patch({ type: value as QuestionType })
                }}
              >
                <SelectTrigger id={`type-${question.key}`} className="w-full" size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TYPES.map((entry) => (
                    <SelectItem key={entry.value} value={entry.value}>
                      {entry.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="min-w-0 flex-1">
              <label className="mb-1 block text-xs text-muted-foreground" htmlFor={`name-${question.key}`}>
                Name
              </label>
              <Input
                id={`name-${question.key}`}
                value={question.name}
                onChange={(event) => patch({ name: event.target.value })}
                placeholder="category"
                className="font-mono text-xs"
                spellCheck={false}
                aria-invalid={invalid}
              />
            </div>
          </div>

          <div>
            <label
              className="mb-1 block text-xs text-muted-foreground"
              htmlFor={`instructions-${question.key}`}
            >
              Instructions
            </label>
            <Textarea
              id={`instructions-${question.key}`}
              value={question.instructions}
              onChange={(event) => patch({ instructions: event.target.value })}
              placeholder="Which team owns this ticket?"
              rows={2}
              className="resize-none text-xs"
              aria-invalid={invalid}
            />
            <p className="mt-1 text-[0.7rem] text-muted-foreground">{type.hint}</p>
          </div>

          {question.type === "choice" ? (
            <KeyValueEditor
              legend="Options"
              rows={question.options}
              onChange={(options) => patch({ options })}
              addLabel="Add option"
              makeRow={() => ({ key: nextKey("o"), text: "" })}
              invalid={invalid}
            />
          ) : null}

          {question.type === "noul" ? (
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label
                  className="mb-1 block text-xs text-muted-foreground"
                  htmlFor={`false-${question.key}`}
                >
                  Outcome when false
                </label>
                <Input
                  id={`false-${question.key}`}
                  value={question.outcomeFalse}
                  onChange={(event) => patch({ outcomeFalse: event.target.value })}
                  placeholder="not spam"
                  className="text-xs"
                  aria-invalid={invalid}
                />
              </div>
              <div>
                <label
                  className="mb-1 block text-xs text-muted-foreground"
                  htmlFor={`true-${question.key}`}
                >
                  Outcome when true
                </label>
                <Input
                  id={`true-${question.key}`}
                  value={question.outcomeTrue}
                  onChange={(event) => patch({ outcomeTrue: event.target.value })}
                  placeholder="spam"
                  className="text-xs"
                  aria-invalid={invalid}
                />
              </div>
            </div>
          ) : null}

          {question.type === "score" ? (
            <ListEditor
              legend="Scale"
              hint="Ordered from lowest to highest. Drag the handle, or use the arrows."
              rows={question.scale}
              onChange={(scale) => patch({ scale })}
              addLabel="Add level"
              placeholder="text shown to the model"
              invalid={invalid}
            />
          ) : null}

          <div className="flex items-center justify-between border-t pt-2">
            <span className="text-[0.7rem] text-muted-foreground">
              {question.type === "choice"
                ? "The answer is keyed by the option key, so keep it stable."
                : question.type === "score"
                  ? "A scale is positional: level 0 is the first row."
                  : "Both outcomes are shown to the model as-is."}
            </span>
            <div className="flex gap-1">
              <Button
                variant="outline"
                size="xs"
                onClick={() => onMove(-1)}
                disabled={index === 0}
              >
                Up
              </Button>
              <Button
                variant="outline"
                size="xs"
                onClick={() => onMove(1)}
                disabled={index === total - 1}
              >
                Down
              </Button>
            </div>
          </div>

          {problems && problems.length > 0 ? (
            <ul className="space-y-1 rounded-md bg-destructive/10 p-2 text-xs text-destructive">
              {problems.map((problem) => (
                <li key={problem}>{problem}</li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
