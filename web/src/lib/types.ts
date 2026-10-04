export type QuestionType = "choice" | "noul" | "score"

export interface Outcomes {
  false: string
  true: string
}

export interface OptionInput {
  key: string
  text: string
}

export interface QuestionInput {
  name: string
  type: QuestionType
  instructions?: string
  options?: Record<string, string>
  options_list?: OptionInput[]
  outcomes?: Outcomes
  scale?: string[]
}

export interface ConfigPatch {
  provider?: string
  base_url?: string
  api_key?: string
  model?: string
  timeout?: number
  retries?: number
}

export interface DecideRequest {
  config?: ConfigPatch
  state: unknown
  questions: QuestionInput[]
  model?: string
  keep_alive?: number
  session_id?: string
  user?: string
  trace?: Record<string, string>
  extra?: Record<string, unknown>
}

export interface OptionView {
  key: string
  text: string
}

export interface QuestionView {
  name: string
  type: QuestionType
  instructions?: string
  options?: OptionView[]
  scale?: string[]
  outcomes?: Outcomes
}

export interface Bar {
  key: string
  text: string
  probability: number
  percent: number
  best: boolean
}

export interface AnswerView {
  name: string
  type: QuestionType
  label: string
  question: QuestionView
  key?: string
  text?: string
  probabilities?: Bar[]
  confidence?: number
  margin?: number
  score?: number
  level?: number
  max?: number
  probability?: number
  true?: boolean
  true_label?: string
  false_label?: string
  summary: string
}

export interface Usage {
  input_tokens?: number
  output_tokens?: number
  total_tokens?: number
  cost?: number
}

export interface DecideResult {
  model?: string
  raw?: unknown
  answers?: Record<string, unknown>
  usage?: Usage
}

export interface DecideResponse {
  result?: DecideResult
  raw?: unknown
  answers: AnswerView[]
  questions: QuestionView[]
  missing?: string[]
  duration_ms?: number
  provider?: string
  model?: string
}

export interface CapabilityView {
  provider: string
  images?: boolean
  max_questions?: number
  max_choices?: number
  max_state_bytes?: number
  known: boolean
}

export interface ValidateResponse {
  ok: boolean
  problems?: string[]
  fields?: string[]
  questions?: QuestionView[]
  capabilities: CapabilityView
}

export interface LimitView {
  min_criteria: number
  max_criteria: number
  max_state_bytes: number
  max_image_bytes: number
}

export interface ConfigView {
  provider: string
  base_url: string
  model?: string
  timeout: number
  retries: number
  api_key_set: boolean
  providers: string[]
  default_model?: string
  limits: LimitView
}

export interface ModelView {
  name: string
  size?: string
  decision: boolean
  vision?: boolean
  family?: string
  context?: string
}

export interface ModelsView {
  provider: string
  base_url: string
  models: ModelView[]
  server_version?: string
  warning?: string
}

export interface ErrorView {
  error: string
  kind?: string
  problems?: string[]
  fields?: string[]
  provider?: string
  status?: number
  retry_after?: number
  retryable?: boolean
}

export interface HistoryRun {
  id: string
  at: number
  provider?: string
  model?: string
  ok: boolean
  duration_ms: number

  body: unknown

  response?: unknown
  error?: ErrorView
  questions?: string[]
  summary?: string
}

export interface HistoryView {
  runs: HistoryRun[]
}
