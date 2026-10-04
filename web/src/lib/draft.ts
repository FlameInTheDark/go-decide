import type { DecideRequest, QuestionInput, QuestionType, QuestionView } from "./types"

export interface DraftQuestion {
  key: string
  name: string
  type: QuestionType
  instructions: string
  options: { key: string; text: string }[]
  outcomeFalse: string
  outcomeTrue: string
  scale: string[]
}

let counter = 0

export function nextKey(prefix: string): string {
  counter += 1
  return `${prefix}-${counter}`
}

export function newQuestion(type: QuestionType = "choice"): DraftQuestion {
  return {
    key: nextKey("q"),
    name: "",
    type,
    instructions: "",
    options: [
      { key: "a", text: "" },
      { key: "b", text: "" },
    ],
    outcomeFalse: "no",
    outcomeTrue: "yes",
    scale: ["", ""],
  }
}

export function starterQuestions(): DraftQuestion[] {
  const first = newQuestion("choice")
  first.name = "category"
  first.instructions = "Which team owns this ticket?"
  first.options = [
    { key: "billing", text: "Billing" },
    { key: "bug", text: "Bug" },
    { key: "account", text: "Account" },
  ]

  const second = newQuestion("score")
  second.name = "urgency"
  second.instructions = "How urgent is this ticket?"
  second.scale = ["Whenever", "Soon", "Blocking revenue right now"]

  return [first, second]
}

const defaultState = {
  title: "Checkout returns 500",
  body: "Every card payment on the pricing page fails with a 500 since this morning.",
}

export function starterRequest(questions: DraftQuestion[]): DecideRequest {
  return {
    state: defaultState,
    questions: toQuestionInputs(questions),
  }
}

function toQuestionInputs(questions: DraftQuestion[]): QuestionInput[] {
  return questions
    .filter((question) => question.name.trim() !== "")
    .map((question) => toQuestionInput(question))
}

export function toQuestionInput(question: DraftQuestion): QuestionInput {
  const input: QuestionInput = {
    name: question.name.trim(),
    type: question.type,
    instructions: question.instructions.trim(),
  }

  if (question.type === "choice") {
    input.options_list = question.options
      .map((option) => ({ key: option.key.trim(), text: option.text.trim() }))
      .filter((option) => option.key !== "" && option.text !== "")
  } else if (question.type === "noul") {
    input.outcomes = {
      false: question.outcomeFalse.trim(),
      true: question.outcomeTrue.trim(),
    }
  } else {
    input.scale = question.scale.map((text) => text.trim()).filter((text) => text !== "")
  }

  return input
}

export function buildRequest(
  state: unknown,
  questions: DraftQuestion[],
  config?: Record<string, unknown>,
): DecideRequest {
  const request: DecideRequest = { state, questions: toQuestionInputs(questions) }
  if (config && Object.keys(config).length > 0) {
    request.config = config
  }
  return request
}

export function fromQuestionView(view: QuestionView): DraftQuestion {
  const draft = newQuestion(view.type)
  draft.name = view.name
  draft.instructions = view.instructions ?? ""

  if (view.options?.length) {
    draft.options = view.options.map((option) => ({ key: option.key, text: option.text }))
  } else if (view.scale?.length) {
    draft.scale = [...view.scale]
  }
  if (view.outcomes) {
    draft.outcomeFalse = view.outcomes.false
    draft.outcomeTrue = view.outcomes.true
  }
  return draft
}

export function toJSON(request: DecideRequest): string {
  return JSON.stringify(request, null, 2)
}

export function fromJSON(text: string): { request?: DecideRequest; problems: string[] } {
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch (err) {
    return { problems: [`the JSON is not valid: ${(err as Error).message}`] }
  }

  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return { problems: ["the request must be a JSON object"] }
  }

  const request = parsed as DecideRequest
  if (!Array.isArray(request.questions)) {
    return { problems: ['"questions" must be a list'] }
  }
  return { request, problems: [] }
}
