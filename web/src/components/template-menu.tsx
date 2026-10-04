import { FileText, Pencil } from "lucide-react"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { templates, type Template } from "@/lib/templates"

const labels: Record<string, string> = Object.fromEntries(templates.map((t) => [t.id, t.label]))

export function TemplateMenu({
  current,
  onPick,
}: {
  current: string | null
  onPick: (template: Template) => void
}) {
  return (
    <Select
      items={labels}
      value={current ?? undefined}
      onValueChange={(value) => {
        const template = templates.find((entry) => entry.id === value)
        if (template) onPick(template)
      }}
    >
      <SelectTrigger size="sm" className="max-w-56" aria-label="Load a template">
        <SelectValue placeholder="Load a template" />
      </SelectTrigger>
      <SelectContent className="w-80" align="start">
        {templates.map((template) => (
          <SelectItem key={template.id} value={template.id}>
            <span className="flex flex-col gap-0.5">
              <span className="flex items-center gap-2">
                {template.id === "blank" ? <Pencil className="size-3 shrink-0" /> : <FileText className="size-3 shrink-0" />}
                <span className="font-medium">{template.label}</span>
              </span>
              <span className="text-xs leading-snug text-muted-foreground">{template.summary}</span>
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}