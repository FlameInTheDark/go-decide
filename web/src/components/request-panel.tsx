import { lazy, Suspense, useMemo, useState } from "react"
import { AlertTriangle, Braces, Check, Plus } from "lucide-react"

import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { QuestionEditor } from "@/components/question-editor"
import type { DraftQuestion } from "@/lib/draft"
import { fromJSON, newQuestion, starterRequest, toJSON } from "@/lib/draft"
import type { DecideRequest } from "@/lib/types"
import { cn } from "cn"

const JsonEditor = lazy(() =>
  import("@/components/json-editor").then((module) => ({ default: module.JsonEditor })),
)

interface RequestPanelProps {
  questions: DraftQuestion[]
  state: string
  onQuestionsChange: (questions: DraftQuestion[]) => void
  onStateChange: (state: string) => void
  onRequestChange: (request: DecideRequest) => void
  request: DecideRequest | null
  problems: string[]
  limits?: { max_questions?: number }
}

export function RequestPanel({
  questions,
  state,
  onQuestionsChange,
  onStateChange,
  onRequestChange,
  request,
  problems,
  limits,
}: RequestPanelProps) {
  const [open, setOpen] = useState<string | null>(questions[0]?.key ?? null)
  const [jsonEdited, setJsonEdited] = useState("")
  const [jsonProblems, setJSONProblems] = useState<string[]>([])
  const [editedJSON, setEditedJSON] = useState(false)

  const builderJSON = useMemo(() => toJSON(starterRequest(questions)), [questions])
  const jsonDraft = editedJSON ? jsonEdited : builderJSON

  const stateProblems = problems.filter((problem) => problem.startsWith("state"))
  const stateInvalid = stateProblems.length > 0
  const questionProblems = useMemo(() => {
    const map = new Map<string, string[]>()
    for (const problem of problems) {
      const match = problem.match(/^questions\[(\d+)\]:\s*(.*)$/)
      if (!match) continue
      const list = map.get(match[1]) ?? []
      list.push(match[2])
      map.set(match[1], list)
    }
    return map
  }, [problems])

  const overLimit = limits?.max_questions ? questions.length > limits.max_questions : false

  const addQuestion = () => {
    const question = newQuestion()
    onQuestionsChange([...questions, question])
    setOpen(question.key)
  }

  const update = (index: number, next: DraftQuestion) => {
    const copy = [...questions]
    copy[index] = next
    onQuestionsChange(copy)
  }

  const move = (index: number, delta: number) => {
    const target = index + delta
    if (target < 0 || target >= questions.length) return
    const copy = [...questions]
    ;[copy[index], copy[target]] = [copy[target], copy[index]]
    onQuestionsChange(copy)
  }

  const applyJSON = () => {
    const { request: parsed, problems: found } = fromJSON(jsonDraft)
    setJSONProblems(found)
    if (parsed) {
      setEditedJSON(false)
      onRequestChange(parsed)
    }
  }

  return (
    <Tabs defaultValue="builder" className="flex min-h-0 flex-1 flex-col gap-0">
      <div className="flex items-center justify-between px-3 pt-3">
        <TabsList>
          <TabsTrigger value="builder">Builder</TabsTrigger>
          <TabsTrigger value="json">JSON</TabsTrigger>
        </TabsList>
        <Button variant="ghost" size="xs" onClick={addQuestion}>
          <Plus />
          Question
        </Button>
      </div>

      <TabsContent value="builder" className="min-h-0 flex-1 px-3 pb-3">
        <ScrollArea className="h-full">
          <div className="space-y-3 pr-3">
            <div>
              <div className="mb-1 flex items-center justify-between">
                <label htmlFor="state" className="text-xs font-medium">
                  State
                </label>
                <span className="text-[0.7rem] text-muted-foreground">
                  text, JSON object or array
                </span>
              </div>
              <Textarea
                id="state"
                value={state}
                onChange={(event) => onStateChange(event.target.value)}
                rows={6}
                spellCheck={false}
                className={cn("font-mono text-xs", stateInvalid && "border-destructive")}
                placeholder='{"title": "Checkout returns 500"}'
              />
              {stateProblems.map((problem) => (
                <p key={problem} className="mt-1 text-xs text-destructive">
                  {problem}
                </p>
              ))}
            </div>

            <div className="space-y-2">
              {questions.map((question, index) => (
                <QuestionEditor
                  key={question.key}
                  question={question}
                  index={index}
                  total={questions.length}
                  invalid={questionProblems.has(String(index))}
                  problems={questionProblems.get(String(index))}
                  expanded={open === question.key}
                  onToggle={() => setOpen(open === question.key ? null : question.key)}
                  onChange={(next) => update(index, next)}
                  onMove={(delta) => move(index, delta)}
                  onDuplicate={() => {
                    const copy: DraftQuestion = { ...question, key: `${question.key}-copy-${Date.now()}` }
                    const next = [...questions]
                    next.splice(index + 1, 0, copy)
                    onQuestionsChange(next)
                  }}
                  onRemove={() => {
                    if (questions.length < 2) return
                    onQuestionsChange(questions.filter((_, i) => i !== index))
                  }}
                />
              ))}
            </div>

            {overLimit ? (
              <p className="flex items-center gap-1.5 text-xs text-destructive">
                <AlertTriangle className="size-3.5" />
                This provider accepts at most {limits?.max_questions} questions.
              </p>
            ) : null}
          </div>
        </ScrollArea>
      </TabsContent>

      <TabsContent value="json" className="min-h-0 flex-1 px-3 pb-3">
        <div className="flex h-full flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
              <Braces className="size-3.5" />
              Same body the CLI takes with --questions
            </span>
            <Button size="xs" onClick={applyJSON}>
              <Check />
              Apply
            </Button>
          </div>
          <Suspense
            fallback={
              <div className="flex min-h-64 flex-1 items-center justify-center text-xs text-muted-foreground">
                Loading the JSON editor...
              </div>
            }
            >
              <JsonEditor
                value={jsonDraft}
                onChange={(next) => {
                  setJsonEdited(next)
                  setEditedJSON(true)
                  setJSONProblems([])
                }}
              />
            </Suspense>
          {jsonProblems.map((problem) => (
            <p key={problem} className="text-xs text-destructive">
              {problem}
            </p>
          ))}
          {jsonProblems.length === 0 && editedJSON ? (
            <p className="text-[0.7rem] text-muted-foreground">
              Apply to send this payload, or switch to Builder to edit it as a form.
            </p>
          ) : null}
          {jsonProblems.length === 0 && !editedJSON && request ? (
            <p className="text-[0.7rem] text-muted-foreground">
              Showing the request the builder produces. That is what Run will send.
            </p>
          ) : null}
        </div>
      </TabsContent>
    </Tabs>
  )
}
