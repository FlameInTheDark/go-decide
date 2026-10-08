import { lazy, Suspense, useMemo, useRef, useState } from "react"
import { AlertTriangle, Braces, Check, Image, Plus, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { QuestionEditor } from "@/components/question-editor"
import type { DraftQuestion } from "@/lib/draft"
import { fromJSON, newQuestion, starterRequest, toJSON } from "@/lib/draft"
import type { DecideRequest, ImageInput } from "@/lib/types"
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
  limits?: { max_questions?: number; max_image_bytes?: number }
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
  const [images, setImages] = useState<ImageInput[]>(request?.images ?? [])
  const [imageErrors, setImageErrors] = useState<string[]>([])
  const fileInputRef = useRef<HTMLInputElement>(null)

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
  const maxImageBytes = limits?.max_image_bytes ?? 32 << 20 // 32 MiB default

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

  // Helper to convert File to base64
  const fileToBase64 = (file: File): Promise<string> => {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result as string
        // Remove data URL prefix (e.g., "data:image/png;base64,")
        const base64 = result.split(",")[1] ?? result
        resolve(base64)
      }
      reader.onerror = reject
      reader.readAsDataURL(file)
    })
  }

  const handleImageFiles = (files: FileList | null) => {
    if (!files) return
    const newErrors: string[] = []
    const newImages: ImageInput[] = []

    for (const file of Array.from(files)) {
      if (!file.type.startsWith("image/")) {
        newErrors.push(`${file.name}: not an image`)
        continue
      }
      if (file.size > maxImageBytes) {
        newErrors.push(`${file.name}: exceeds ${Math.round(maxImageBytes / 1024 / 1024)} MiB limit`)
        continue
      }
      fileToBase64(file).then((base64) => {
        newImages.push({ base64, mime_type: file.type })
        if (newImages.length === Array.from(files).length) {
          if (newErrors.length > 0) {
            setImageErrors(newErrors)
          }
          setImages((prev) => [...prev, ...newImages])
          // If no manual JSON request exists, build from the builder state
          const baseRequest = request ?? starterRequest(questions)
          onRequestChange({ ...baseRequest, images: [...(baseRequest.images ?? []), ...newImages] })
        }
      }).catch(() => {
        newErrors.push(`${file.name}: failed to read`)
        if (newErrors.length > 0 && newImages.length + newErrors.length === Array.from(files).length) {
          setImageErrors(newErrors)
        }
      })
    }
  }

  const removeImage = (index: number) => {
    setImages((prev) => prev.filter((_, i) => i !== index))
    const baseRequest = request ?? starterRequest(questions)
    onRequestChange({ ...baseRequest, images: baseRequest.images?.filter((_, i) => i !== index) })
  }

  return (
    <Tabs defaultValue="builder" className="flex min-h-0 flex-1 flex-col gap-0">
      <div className="flex items-center justify-between px-3 pt-3">
        <TabsList>
          <TabsTrigger value="builder">Builder</TabsTrigger>
          <TabsTrigger value="images">Images</TabsTrigger>
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

      <TabsContent value="images" className="min-h-0 flex-1 px-3 pb-3">
        <div className="flex h-full flex-col gap-3">
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
              <Image className="size-3.5" />
              Images shared by all questions. Requires a vision-capable model.
            </span>
            <Button
              variant="outline"
              size="xs"
              onClick={() => fileInputRef.current?.click()}
            >
              <Plus />
              Add images
            </Button>
          </div>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            multiple
            onChange={(e) => handleImageFiles(e.target.files)}
            className="hidden"
          />
          {imageErrors.length > 0 && (
            <div className="space-y-1">
              {imageErrors.map((err, i) => (
                <p key={i} className="text-xs text-destructive flex items-center gap-1.5">
                  <AlertTriangle className="size-3.5" />
                  {err}
                </p>
              ))}
            </div>
          )}
          {images.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {images.map((img, i) => (
                <div key={i} className="relative rounded border bg-card p-1 flex items-center gap-2">
                  <img
                    src={`data:${img.mime_type ?? "image/png"};base64,${img.base64}`}
                    alt={`Image ${i + 1}`}
                    className="h-16 w-16 object-cover rounded"
                  />
                  <div className="flex flex-col text-[0.7rem] text-muted-foreground">
                    <span>Image {i + 1}</span>
                    <span>{img.mime_type ?? "unknown"}</span>
                    <span>
                      {Math.round((img.base64.length * 3) / 4 / 1024)} KiB
                    </span>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => removeImage(i)}
                    className="ml-auto text-destructive hover:text-destructive"
                    aria-label={`Remove image ${i + 1}`}
                  >
                    <X className="size-3.5" />
                  </Button>
                </div>
              ))}
            </div>
          )}
          {images.length === 0 && imageErrors.length === 0 && (
            <div className="flex flex-col items-center justify-center gap-2 py-8 text-center text-muted-foreground">
              <Image className="size-8 opacity-50" />
              <p className="text-sm">No images attached</p>
              <p className="max-w-xs text-[0.7rem]">
                Add images to let the model see visual content. Each image is
                shared across all questions.
              </p>
            </div>
          )}
        </div>
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
