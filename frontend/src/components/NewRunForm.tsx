import { AlertCircle, Check, Plus } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ApiError } from '@/api/client'
import { useCreateRun } from '@/api/hooks'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { RunVariant } from '@/types'

// `echo` and `http_get` are the tools in the platform registry (http_get is the
// SSRF-guarded fetch). `fs_read` and `clock` are deliberately NOT registered, to
// demonstrate the least-privilege model: a request for a tool outside the
// registry is denied and audited.
const TOOL_OPTIONS = ['echo', 'http_get', 'fs_read', 'clock'] as const

function ToolChip({
  tool,
  selected,
  onToggle,
}: {
  tool: string
  selected: boolean
  onToggle: () => void
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={selected}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-3 py-1 font-mono text-xs transition-colors',
        selected
          ? 'border-primary/40 bg-primary/15 text-primary'
          : 'border-border text-muted-foreground hover:bg-accent hover:text-foreground',
      )}
    >
      {selected && <Check className="size-3" aria-hidden />}
      {tool}
    </button>
  )
}

export function NewRunForm() {
  const navigate = useNavigate()
  const create = useCreateRun()
  const [objective, setObjective] = useState('')
  const [variant, setVariant] = useState<RunVariant>('custom')
  const [tools, setTools] = useState<string[]>(['echo'])

  const toggleTool = (tool: string) =>
    setTools((prev) => (prev.includes(tool) ? prev.filter((t) => t !== tool) : [...prev, tool]))

  const trimmed = objective.trim()
  const canSubmit = trimmed.length > 0 && !create.isPending

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    create.mutate(
      { objective: trimmed, variant, allowedTools: tools.length > 0 ? tools : ['echo'] },
      { onSuccess: (run) => navigate(`/runs/${run.id}`) },
    )
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="objective">Objective</Label>
        <Textarea
          id="objective"
          value={objective}
          onChange={(e) => setObjective(e.target.value)}
          placeholder="e.g. Summarise the incoming request and echo the key details back."
          rows={3}
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="variant">Agent variant</Label>
        <Select
          id="variant"
          value={variant}
          onChange={(e) => setVariant(e.target.value as RunVariant)}
        >
          <option value="custom">custom — hand-written plan → act → observe loop</option>
          <option value="langgraph">langgraph — LangGraph StateGraph variant</option>
        </Select>
      </div>

      <div className="space-y-2">
        <Label>Allowed tools</Label>
        <div className="flex flex-wrap gap-2">
          {TOOL_OPTIONS.map((tool) => (
            <ToolChip
              key={tool}
              tool={tool}
              selected={tools.includes(tool)}
              onToggle={() => toggleTool(tool)}
            />
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          The run is scoped to exactly these tools. Requests for anything outside the platform
          registry are denied and audited — <span className="font-mono">echo</span> is wired in the
          default profile.
        </p>
      </div>

      {create.isError && (
        <div className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
          <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden />
          <span>
            {create.error instanceof ApiError ? create.error.message : 'Failed to create run.'}
          </span>
        </div>
      )}

      <Button type="submit" disabled={!canSubmit} className="w-full sm:w-auto">
        {create.isPending ? <Spinner /> : <Plus className="size-4" />}
        {create.isPending ? 'Creating…' : 'Create run'}
      </Button>
    </form>
  )
}
