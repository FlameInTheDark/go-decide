import type {
  ConfigPatch,
  ConfigView,
  DecideRequest,
  DecideResponse,
  ErrorView,
  HistoryView,
  ModelsView,
  ValidateResponse,
} from "./types"

const GUARD_HEADER = "X-Decide-Playground"

export class ApiError extends Error {
  readonly status: number
  readonly view: ErrorView

  constructor(status: number, view: ErrorView) {
    super(view.error)
    this.name = "ApiError"
    this.status = status
    this.view = view
  }
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      [GUARD_HEADER]: "1",
      ...init?.headers,
    },
  })

  if (response.status === 204) {
    return undefined as T
  }

  const text = await response.text()
  let body: unknown
  try {
    body = text === "" ? undefined : JSON.parse(text)
  } catch {
    throw new ApiError(response.status, { error: text || response.statusText })
  }

  if (!response.ok) {
    const view = (body as { error?: ErrorView } | undefined)?.error
    throw new ApiError(response.status, view ?? { error: response.statusText })
  }
  return body as T
}

export const api = {
  config: () => call<ConfigView>("/api/config"),

  models: (config: ConfigPatch) => {
    const query = new URLSearchParams()
    if (config.provider) query.set("provider", config.provider)
    if (config.base_url) query.set("base_url", config.base_url)
    const suffix = query.toString()
    return call<ModelsView>(`/api/models${suffix ? `?${suffix}` : ""}`)
  },

  validate: (request: DecideRequest) =>
    call<ValidateResponse>("/api/validate", { method: "POST", body: JSON.stringify(request) }),

  decide: (request: DecideRequest) =>
    call<DecideResponse>("/api/decide", { method: "POST", body: JSON.stringify(request) }),

  history: () => call<HistoryView>("/api/history"),

  clearHistory: () => call<void>("/api/history/clear", { method: "POST" }),
}
