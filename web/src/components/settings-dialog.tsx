import { useState } from "react"
import { KeyRound, RefreshCw, Settings2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { ConfigPatch, ConfigView, ModelsView } from "@/lib/types"

interface SettingsDialogProps {
  config: ConfigView | null
  patch: ConfigPatch
  models: ModelsView | null
  modelsError: string | null
  onPatch: (patch: ConfigPatch) => void
  reloadModels: () => Promise<ModelsView | null>
}

export function SettingsDialog({ config, patch, models, modelsError, onPatch, reloadModels }: SettingsDialogProps) {
  const [open, setOpen] = useState(false)
  const [key, setKey] = useState("")

  const provider = patch.provider ?? config?.provider ?? "ollama"
  const baseURL = patch.base_url ?? config?.base_url ?? ""
  const model = patch.model ?? config?.model ?? ""

  const update = (fields: Partial<ConfigPatch>) => onPatch({ ...patch, ...fields })

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setKey("")
      }}
    >
      <DialogTrigger
        render={
          <Button variant="ghost" size="icon-sm" aria-label="Provider settings" />
        }
      >
        <Settings2 />
      </DialogTrigger>

      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Provider settings</DialogTitle>
          <DialogDescription>
            These are per-browser settings. The server keeps them in memory and never writes
            them to disk.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="provider" className="mb-1 block text-xs">
                Provider
              </Label>
              <Select
                value={provider}
                onValueChange={(value) => value && update({ provider: value })}
              >
                <SelectTrigger id="provider" className="w-full" size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(config?.providers ?? []).map((name) => (
                    <SelectItem key={name} value={name}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-end gap-2">
              <Label htmlFor="model" className="mb-1 block text-xs">
                Model
              </Label>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={reloadModels}
                disabled={reloadModels === undefined}
                aria-label="Reload available models"
              >
                <RefreshCw className="size-3.5" />
              </Button>
              {models && models.models.length > 0 ? (
                <Select value={model} onValueChange={(value) => value && update({ model: value })}>
                  <SelectTrigger id="model" className="w-full" size="sm">
                    <SelectValue placeholder="default" />
                  </SelectTrigger>
                  <SelectContent>
                    {models.models.map((entry) => (
                      <SelectItem key={entry.name} value={entry.name}>
                        {entry.name}
                        {entry.decision ? "" : " (no decision support)"}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  id="model"
                  value={model}
                  onChange={(event) => update({ model: event.target.value })}
                  placeholder={config?.default_model ?? "provider default"}
                  className="font-mono text-xs"
                />
              )}
            </div>
          </div>

          <div>
            <Label htmlFor="base-url" className="mb-1 block text-xs">
              Base URL
            </Label>
            <Input
              id="base-url"
              value={baseURL}
              onChange={(event) => update({ base_url: event.target.value })}
              placeholder="http://localhost:11434"
              className="font-mono text-xs"
            />
            {modelsError ? (
              <p className="mt-1 text-[0.7rem] text-destructive">{modelsError}</p>
            ) : null}
            {models?.warning ? (
              <p className="mt-1 text-[0.7rem] text-destructive">{models.warning}</p>
            ) : null}
          </div>

          {provider === "openrouter" ? (
            <div>
              <Label htmlFor="api-key" className="mb-1 block text-xs">
                API key
              </Label>
              <Input
                id="api-key"
                type="password"
                value={key}
                onChange={(event) => setKey(event.target.value)}
                onBlur={() => key && update({ api_key: key })}
                placeholder={config?.api_key_set ? "•••••••• (set)" : "sk-or-v1-..."}
                className="font-mono text-xs"
              />
              <p className="mt-1 inline-flex items-center gap-1 text-[0.7rem] text-muted-foreground">
                <KeyRound className="size-3" />
                {config?.api_key_set ? "A key is set on the server." : "No key is set."} It stays in
                the browser's memory and is sent only with each request.
              </p>
            </div>
          ) : null}

          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="timeout" className="mb-1 block text-xs">
                Timeout (s)
              </Label>
              <Input
                id="timeout"
                type="number"
                min={1}
                value={patch.timeout ?? config?.timeout ?? 120}
                onChange={(event) => update({ timeout: Number(event.target.value) })}
                className="text-xs"
              />
            </div>
            <div>
              <Label htmlFor="retries" className="mb-1 block text-xs">
                Retries
              </Label>
              <Input
                id="retries"
                type="number"
                min={0}
                value={patch.retries ?? config?.retries ?? 2}
                onChange={(event) => update({ retries: Number(event.target.value) })}
                className="text-xs"
              />
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button size="sm" onClick={() => setOpen(false)}>
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
