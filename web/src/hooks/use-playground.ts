import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import { ApiError, api } from "@/lib/api"
import type {
  ConfigPatch,
  ConfigView,
  DecideRequest,
  DecideResponse,
  ErrorView,
  HistoryRun,
  ModelsView,
  QuestionView,
  ValidateResponse,
} from "@/lib/types"

export function usePlayground() {
  const [config, setConfig] = useState<ConfigView | null>(null)
  const [patch, setPatch] = useState<ConfigPatch>({})
  const [models, setModels] = useState<ModelsView | null>(null)
  const [modelsError, setModelsError] = useState<string | null>(null)
  const [validate, setValidate] = useState<ValidateResponse | null>(null)
  const [result, setResult] = useState<DecideResponse | null>(null)
  const [failure, setFailure] = useState<ErrorView | null>(null)
  const [history, setHistory] = useState<HistoryRun[]>([])
  const [running, setRunning] = useState(false)
  const [validated, setValidated] = useState<{ problems: string[]; fields: string[] } | null>(null)

  const stamp = useRef(0)

  const loadConfig = useCallback(async () => {
    const view = await api.config()
    setConfig(view)
    setPatch((current) => ({ ...current, provider: current.provider ?? view.provider }))
    return view
  }, [])

  const loadHistory = useCallback(async () => {
    const view = await api.history()
    setHistory(view.runs ?? [])
    return view.runs ?? []
  }, [])

  const loadModels = useCallback(
    async (next: ConfigPatch) => {
      setModelsError(null)
      try {
        const view = await api.models(next)
        setModels(view)
        return view
      } catch (err) {
        setModels(null)
        setModelsError(err instanceof ApiError ? err.message : String(err))
        return null
      }
    },
    [],
  )

  useEffect(() => {
    const handle = setTimeout(() => {
      loadConfig().catch(() => undefined)
      loadHistory().catch(() => undefined)
    }, 0)
    return () => clearTimeout(handle)
  }, [loadConfig, loadHistory])

  useEffect(() => {
    const provider = patch.provider ?? config?.provider
    const baseURL = patch.base_url ?? config?.base_url
    if (!provider) return
    const handle = setTimeout(() => {
      loadModels({ provider, base_url: baseURL }).catch(() => undefined)
    }, 250)
    return () => clearTimeout(handle)
  }, [patch.provider, patch.base_url, config?.provider, config?.base_url, loadModels])

  const runValidate = useCallback(async (request: DecideRequest) => {
    const ticket = ++stamp.current
    try {
      const view = await api.validate(request)
      if (ticket !== stamp.current) return null
      setValidated({
        problems: view.problems ?? [],
        fields: view.fields ?? [],
      })
      return view
    } catch (err) {
      if (ticket !== stamp.current) return null
      setValidated({
        problems: [err instanceof ApiError ? err.message : String(err)],
        fields: [],
      })
      return null
    }
  }, [])

  const run = useCallback(
    async (request: DecideRequest) => {
      setRunning(true)
      setFailure(null)
      try {
        const response = await api.decide(request)
        setResult(response)
        setValidated({ problems: [], fields: [] })
        return response
      } catch (err) {
        setResult(null)
        if (err instanceof ApiError) {
          setFailure(err.view)
        } else {
          setFailure({ error: String(err) })
        }
        return null
      } finally {
        setRunning(false)

        loadHistory().catch(() => undefined)
      }
    },
    [loadHistory],
  )

  const clearHistory = useCallback(async () => {
    await api.clearHistory()
    setHistory([])
  }, [])

  const restore = useCallback((entry: HistoryRun): DecideRequest | null => {
    const { body } = entry
    if (!body || typeof body !== "object" || Array.isArray(body)) return null
    return body as DecideRequest
  }, [])

  const showRun = useCallback((entry: HistoryRun) => {
    setRunning(false)
    if (!entry.ok || !entry.response) {
      setResult(null)
      setFailure(entry.error ?? { error: entry.summary ?? "this run has no stored response" })
      return null
    }
    try {
      const stored = entry.response as DecideResponse
      if (!stored || typeof stored !== "object" || !Array.isArray(stored.answers)) {
        throw new Error("not a stored response")
      }
      setFailure(null)
      setResult(stored)
      return stored
    } catch {
      setResult(null)
      setFailure({ error: "this run has no readable stored response" })
      return null
    }
  }, [])

  return useMemo(
    () => ({
      config,
      patch,
      setPatch,
      models,
      modelsError,
      validate,
      setValidate,
      result,
      failure,
      running,
      validated,
      history,
      run,
      runValidate,
      clearHistory,
      restore,
      showRun,
      loadModels,
      loadHistory,
    }),
    [
      config,
      patch,
      models,
      modelsError,
      validate,
      result,
      failure,
      running,
      validated,
      history,
      run,
      runValidate,
      clearHistory,
      restore,
      showRun,
      loadModels,
      loadHistory,
    ],
  )
}

export type Playground = ReturnType<typeof usePlayground>

export function problemsOf(playground: Playground): { problems: string[]; fields: string[] } {
  if (playground.failure) {
    return {
      problems: playground.failure.problems?.length ? playground.failure.problems : [playground.failure.error],
      fields: playground.failure.fields ?? [],
    }
  }
  return playground.validated ?? { problems: [], fields: [] }
}

export type { QuestionView }
