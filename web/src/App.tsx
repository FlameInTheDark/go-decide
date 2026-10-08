import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Loader2, RotateCcw, Sparkles } from "lucide-react"

import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { TooltipProvider } from "@/components/ui/tooltip"
import { HistoryPanel } from "@/components/history-panel"
import { RequestPanel } from "@/components/request-panel"
import { ResultsPanel, formatDuration } from "@/components/results-panel"
import { RunBar } from "@/components/run-bar"
import { SettingsDialog } from "@/components/settings-dialog"
import { StatusBar } from "@/components/status-bar"
import { problemsOf, usePlayground } from "@/hooks/use-playground"
import type { DraftQuestion } from "@/lib/draft"
import { newQuestion, starterQuestions } from "@/lib/draft"
import type { Template } from "@/lib/templates"
import { requestFromTemplate } from "@/lib/templates"
import type { DecideRequest, HistoryRun, QuestionInput } from "@/lib/types"

function toDraft(question: QuestionInput): DraftQuestion {
  const draft = newQuestion(question.type)
  draft.name = question.name
  draft.instructions = question.instructions ?? ""

  if (question.options_list) {
    draft.options = question.options_list.map((option) => ({ key: option.key, text: option.text }))
  } else if (question.options) {
    draft.options = Object.entries(question.options).map(([key, text]) => ({ key, text }))
  }
  if (question.scale) {
    draft.scale = [...question.scale]
  }
  if (question.outcomes) {
    draft.outcomeFalse = question.outcomes.false
    draft.outcomeTrue = question.outcomes.true
  }
  return draft
}

function requestFrom(questions: DraftQuestion[], state: string, raw?: DecideRequest | null): DecideRequest {
  if (raw) return raw
  let parsed: unknown = state
  const trimmed = state.trim()
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    try {
      parsed = JSON.parse(trimmed)
    } catch {
      parsed = state
    }
  }

  return {
    state: parsed,
    questions: questions
      .filter((question) => question.name.trim() !== "")
      .map((question) => toInput(question)),
  }
}

function toInput(question: DraftQuestion): QuestionInput {
  const input: QuestionInput = {
    name: question.name.trim(),
    type: question.type,
    instructions: question.instructions.trim(),
  }
  if (question.type === "choice") {
    input.options = Object.fromEntries(
      question.options
        .map((option) => [option.key.trim(), option.text.trim()] as const)
        .filter(([key, text]) => key !== "" && text !== ""),
    )
  } else if (question.type === "noul") {
    input.outcomes = {
      false: question.outcomeFalse.trim() || "no",
      true: question.outcomeTrue.trim() || "yes",
    }
  } else {
    input.scale = question.scale
      .map((text) => text.trim())
      .filter((text) => text !== "")
  }
  return input
}

export default function App() {
  const playground = usePlayground()
  const {
    config,
    patch,
    setPatch,
    models,
    modelsError,
    result,
    failure,
    running,
    history,
    run,
    runValidate,
    clearHistory,
    restore,
    showRun,
    reloadModels,
  } = playground

  const initial = useMemo(() => starterQuestions(), [])
  const [questions, setQuestions] = useState<DraftQuestion[]>(initial)
  const [state, setState] = useState(
    '{\n  "title": "Checkout returns 500",\n  "body": "Every card payment fails with a 500 since this morning."\n}',
  )
  const [pasted, setPasted] = useState<DecideRequest | null>(null)
  const [historyOpen, setHistoryOpen] = useState(true)
  const [validating, setValidating] = useState(false)

  const [selectedRun, setSelectedRun] = useState<string | null>(null)
  const [template, setTemplate] = useState<string | null>(null)

  const request = useMemo(
    () => requestFrom(questions, state, pasted),
    [questions, state, pasted],
  )

  const setQuestionsAndForget = useCallback((next: DraftQuestion[]) => {
    setPasted(null)
    setQuestions(next)
  }, [])

  const { problems } = problemsOf(playground)

  const timer = useRef<number | undefined>(undefined)
  useEffect(() => {
    window.clearTimeout(timer.current)
    if (running) return
    timer.current = window.setTimeout(() => {
      setValidating(true)
      runValidate(request)
        .then(() => setValidating(false))
        .catch(() => setValidating(false))
    }, 400)
    return () => window.clearTimeout(timer.current)
  }, [request, running, runValidate])

  const start = async () => {
    const body = { ...request, config: patch }
    setSelectedRun(null)
    setStatus(null)
    await run(body)
  }

  const restoreRun = (entry: HistoryRun) => {
    const decoded = restore(entry)
    if (!decoded) return

    setState(JSON.stringify(decoded.state, null, 2))
    const drafts = (decoded.questions ?? []).map(toDraft)
    setQuestions(drafts.length > 0 ? drafts : [newQuestion()])
    setPasted(decoded)
  }

  const selectRun = (entry: HistoryRun) => {
    setSelectedRun(entry.id)
    showRun(entry)
  }

  const clearAll = () => {
    setQuestions([newQuestion()])
    setState("")
    setPasted(null)
    setTemplate(null)
  }

  const applyTemplate = (picked: Template) => {
    const next = requestFromTemplate(picked)
    setState(JSON.stringify(next.state, null, 2))
    const drafts = (next.questions ?? []).map(toDraft)
    setQuestionsAndForget(drafts.length > 0 ? drafts : [newQuestion()])
    setTemplate(picked.id)
    setSelectedRun(null)
  }

  const valid = problems.length === 0 && (questions.length > 0 || pasted !== null)

  const finished = useRef<HistoryRun | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  useEffect(() => {
    const latest = history[0]
    if (!latest || latest.id === finished.current?.id) return
    finished.current = latest
    if (!latest.ok) {
      setStatus(null)
      return
    }
    const names = latest.questions?.length ? latest.questions.join(", ") : ""
    setStatus(
      `${latest.provider ?? "provider"} answered in ${formatDuration(latest.duration_ms)}${
        names ? ` for ${names}` : ""
      }.`,
    )
  }, [history])

  return (
    <TooltipProvider>
      <div className="flex h-dvh flex-col bg-background text-foreground">
        <RunBar
          running={running}
          valid={valid}
          problemCount={problems.length}
          onRun={start}
          onClear={clearAll}
          onTemplate={applyTemplate}
          template={template}
          onToggleHistory={() => setHistoryOpen(!historyOpen)}
          historyOpen={historyOpen}
          provider={patch.provider ?? config?.provider}
          model={patch.model ?? config?.model ?? config?.default_model}
        />

        <div className="flex items-center justify-end gap-1 border-b px-3 py-1">
          {validating ? (
            <span className="inline-flex items-center gap-1 text-[0.7rem] text-muted-foreground">
              <Loader2 className="size-3 animate-spin" />
              checking
            </span>
          ) : null}
          {models && models.models.length > 0 ? (
            <span className="inline-flex items-center gap-1 text-[0.7rem] text-muted-foreground">
              <Sparkles className="size-3" />
              {models.models.filter((entry) => entry.decision).length} decision models available
            </span>
          ) : null}
          <SettingsDialog config={config} patch={patch} models={models} modelsError={modelsError} onPatch={setPatch} reloadModels={reloadModels} />
        </div>

        <div className="flex min-h-0 flex-1">
          <section className="flex min-w-0 flex-1 flex-col">
            <div className="flex min-h-0 flex-1">
              <div className="flex min-w-0 flex-1 flex-col border-r">
                <RequestPanel
                  questions={questions}
                  state={state}
                  onQuestionsChange={setQuestionsAndForget}
                  onStateChange={(value) => {
                    setPasted(null)
                    setState(value)
                  }}
                  onRequestChange={setPasted}
                  request={pasted}
                  problems={problems}
                  limits={{
                    max_questions: playground.validate?.capabilities?.max_questions,
                    max_image_bytes: playground.validate?.capabilities?.max_state_bytes,
                  }}
                />
              </div>

              <Separator orientation="vertical" className="h-full" />

              <div className="flex min-w-0 flex-1 flex-col">
                <header className="flex h-9 shrink-0 items-center gap-2 border-b px-3">
                  <span className="text-xs font-medium">Result</span>
                  {result?.duration_ms !== undefined ? (
                    <span className="text-[0.7rem] text-muted-foreground">
                      {result.answers.length} answer{result.answers.length === 1 ? "" : "s"}
                    </span>
                  ) : null}
                </header>
                <ScrollArea className="min-h-0 flex-1">
                  <div className="p-3">
                    {running ? (
                      <div className="flex h-full flex-col items-center justify-center gap-2 py-16 text-muted-foreground">
                        <Loader2 className="size-5 animate-spin" />
                        <p className="text-xs">Waiting for the model…</p>
                      </div>
                    ) : result ? (
                      <ResultsPanel response={result} />
                    ) : failure ? (
                      <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-3">
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            <p className="text-sm font-medium text-destructive">{failure.error}</p>
                            {failure.kind ? (
                              <p className="mt-1 font-mono text-[0.7rem] text-muted-foreground">
                                kind={failure.kind}
                                {failure.status ? ` status=${failure.status}` : ""}
                              </p>
                            ) : null}
                          </div>
                          <Button
                            variant="outline"
                            size="xs"
                            onClick={start}
                            disabled={!valid || running}
                          >
                            <RotateCcw />
                            Retry
                          </Button>
                        </div>
                      </div>
                    ) : (
                      <div className="flex flex-col items-center justify-center gap-2 py-16 text-center text-muted-foreground">
                        <p className="text-sm">No decision yet</p>
                        <p className="max-w-xs text-xs">
                          Press Run to send this request. The answers come back with their
                          probabilities, and the raw provider response stays one click away.
                        </p>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={start}
                          disabled={!valid || running}
                        >
                          Run
                        </Button>
                      </div>
                    )}
                  </div>
                </ScrollArea>
              </div>
            </div>
          </section>

          {historyOpen ? (
            <HistoryPanel
              runs={history}
              selected={selectedRun}
              onSelect={selectRun}
              onRestore={restoreRun}
              onClear={clearHistory}
            />
          ) : null}
        </div>

        <StatusBar
          problems={problems}
          checking={validating || running}
          status={status ?? undefined}
        />
      </div>
    </TooltipProvider>
  )
}
