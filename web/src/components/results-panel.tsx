import { AlertTriangle, ChevronDown, CircleCheck, Clock, Coins } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { CodeBlock } from "@/components/code-block"
import type { AnswerView, Bar, DecideResponse } from "@/lib/types"
import { cn } from "cn"

export function ResultsPanel({ response }: { response: DecideResponse }) {
  const usage = response.result?.usage
  const payloadHeight = "24rem"

  return (
    <div className="space-y-4">
      {response.prompt ? (
        <CodeBlock
          label="Prompt"
          hint="the state this run was decided from"
          value={response.prompt}
          maxHeight="20rem"
          collapsible
        />
      ) : null}

      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {response.model ? (
          <span className="font-mono">{response.model}</span>
        ) : null}
        <span className="inline-flex items-center gap-1">
          <Clock />
          {formatDuration(response.duration_ms ?? 0)}
        </span>
        {usage && usage.total_tokens ? (
          <span className="inline-flex items-center gap-1">
            {usage.input_tokens ?? 0} in / {usage.output_tokens ?? 0} out
          </span>
        ) : null}
        {usage && usage.cost ? (
          <span className="inline-flex items-center gap-1">
            <Coins />
            ${usage.cost.toFixed(5)}
          </span>
        ) : null}
      </div>

      {response.missing && response.missing.length > 0 ? (
        <div className="flex items-start gap-2 rounded-lg bg-destructive/10 p-3 text-xs text-destructive">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">The model skipped {response.missing.length} question(s)</p>
            <p className="mt-0.5">{response.missing.join(", ")}</p>
          </div>
        </div>
      ) : null}

      <div className="space-y-3">
        {response.answers.map((answer) => (
          <AnswerCard key={answer.name} answer={answer} />
        ))}
      </div>

      <div className="grid gap-3 lg:grid-cols-2 lg:items-start">
        <CodeBlock
          label="Result (decide.Result)"
          value={response.result}
          maxHeight={payloadHeight}
          collapsible
        />
        <CodeBlock
          label="Raw provider response"
          value={response.raw}
          maxHeight={payloadHeight}
          collapsible
        />
      </div>
    </div>
  )
}

function AnswerCard({ answer }: { answer: AnswerView }) {
  return (
    <section className="rounded-lg border bg-card p-3">
      <header className="mb-2 flex items-center gap-2">
        <h3 className="font-mono text-sm font-medium">{answer.name}</h3>
        <Badge variant="outline" className="font-mono text-[0.65rem] uppercase">
          {answer.type}
        </Badge>
      </header>

      {answer.type === "choice" ? <ChoiceBody answer={answer} /> : null}
      {answer.type === "noul" ? <NoulBody answer={answer} /> : null}
      {answer.type === "score" ? <ScoreBody answer={answer} /> : null}

      <p className="mt-2 border-t pt-2 font-mono text-[0.7rem] text-muted-foreground">
        {answer.summary}
      </p>
    </section>
  )
}

function ChoiceBody({ answer }: { answer: AnswerView }) {
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-lg font-medium">{answer.text || answer.label}</span>
        {answer.confidence ? (
          <span className="text-xs text-muted-foreground">
            confidence {(answer.confidence * 100).toFixed(1)}%
          </span>
        ) : null}
        {answer.margin !== undefined ? (
          <span className="text-xs text-muted-foreground">
            margin {(answer.margin * 100).toFixed(1)}%
          </span>
        ) : null}
      </div>
      <Bars bars={answer.probabilities ?? []} />
    </div>
  )
}

function NoulBody({ answer }: { answer: AnswerView }) {
  const probability = answer.probability ?? 0
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="inline-flex items-center gap-1.5 text-lg font-medium">
          {answer.label}
          <CircleCheck className="size-4 text-muted-foreground" />
        </span>
        <span className="text-xs text-muted-foreground">
          p(true) {(probability * 100).toFixed(2)}%
        </span>
      </div>
      <div className="flex h-2 overflow-hidden rounded-full bg-muted">
        <div
          className="bg-primary transition-[width] duration-300"
          style={{ width: `${(probability * 100).toFixed(2)}%` }}
        />
      </div>
      <div className="flex justify-between text-[0.7rem] text-muted-foreground">
        <span>{answer.false_label}</span>
        <span>{answer.true_label}</span>
      </div>
    </div>
  )
}

function ScoreBody({ answer }: { answer: AnswerView }) {
  const max = answer.max ?? 0
  const level = answer.level ?? 0
  const steps = Math.max(max + 1, 1)
  const score = answer.score ?? 0
  const clamped = Math.min(Math.max(score, 0), max)
  const position = max > 0 ? (clamped / max) * 100 : 50
  const marker = Math.min(Math.max(position, 3), 97)

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-lg font-medium">{answer.label}</span>
        <span className="text-xs text-muted-foreground">
          score {answer.score?.toFixed(4) ?? "0"}
        </span>
        <span className="text-xs text-muted-foreground">
          level {level} of {max}
        </span>
      </div>

      <div className="relative pt-4">
        <div className="flex items-center gap-1" aria-hidden>
          {Array.from({ length: steps }, (_, step) => (
            <span
              key={step}
              className={cn(
                "h-1.5 flex-1 rounded-full",
                step === level ? "bg-emerald-500" : "bg-muted",
              )}
            />
          ))}
        </div>
        <span
          role="img"
          aria-label={`level ${level} of ${max}, score ${answer.score?.toFixed(2) ?? "0"}`}
          className="absolute top-0 flex w-5 justify-center text-emerald-400"
          style={{ left: `${marker.toFixed(2)}%` }}
        >
          <ChevronDown className="size-4" strokeWidth={2.5} />
        </span>
      </div>

      <Bars bars={answer.probabilities ?? []} numbered />
    </div>
  )
}

function Bars({ bars, numbered }: { bars: Bar[]; numbered?: boolean }) {
  if (bars.length === 0) return null
  return (
    <ul className="space-y-1">
      {bars.map((bar) => (
        <li key={bar.key} className="flex items-center gap-2 text-xs">
          {numbered ? (
            <span
              className={cn(
                "w-4 shrink-0 text-right font-mono",
                bar.best ? "text-emerald-400" : "text-muted-foreground/70",
              )}
            >
              {bar.key}
            </span>
          ) : null}
          <span
            className={cn(
              "flex w-20 shrink-0 items-center gap-1 truncate",
              bar.best ? "font-medium text-emerald-400" : "text-muted-foreground",
            )}
            title={bar.text || bar.key}
          >
            {bar.best ? <CircleCheck className="size-3 shrink-0" /> : null}
            <span className="truncate">{bar.text || bar.key}</span>
          </span>
          <span className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-muted">
            <span
              className={cn(
                "block h-full rounded-full transition-[width] duration-300",
                bar.best ? "bg-emerald-500" : "bg-muted-foreground/40",
              )}
              style={{ width: `${Math.max(bar.percent, 0.5).toFixed(2)}%` }}
            />
          </span>
          <span
            className={cn(
              "w-12 shrink-0 text-right tabular-nums",
              bar.best ? "text-emerald-400" : "text-muted-foreground",
            )}
          >
            {bar.percent.toFixed(2)}%
          </span>
        </li>
      ))}
    </ul>
  )
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)} s`
  return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`
}
