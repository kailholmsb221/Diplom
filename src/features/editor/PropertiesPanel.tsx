import { type Dispatch } from 'react'
import { Copy, RotateCcw, Scissors, Trash2, Volume2, VolumeX, Wand2 } from 'lucide-react'
import { Button } from '@/shared/ui/Button'
import { cn } from '@/shared/lib/cn'
import type { ClipColor, TimelineClip } from '@/shared/types'
import { COLOR_PARAMS, COLOR_PRESETS, NEUTRAL_COLOR, withNeutralColor } from './editorDefaults'
import { type EngineAction } from './useEditorEngine'

interface PropertiesPanelProps {
  clip: TimelineClip | null
  isAudio: boolean
  dispatch: Dispatch<EngineAction>
}

export function PropertiesPanel({ clip, isAudio, dispatch }: PropertiesPanelProps) {
  if (!clip) {
    return (
      <div className="p-4 text-sm text-muted">
        Выберите клип на таймлайне, чтобы настроить обрезку, цвет и звук.
      </div>
    )
  }

  const color = withNeutralColor(clip.color)
  const audio = clip.audio ?? { muted: false, volume: 1, fadeIn: 0, fadeOut: 0 }

  return (
    <div className="flex flex-col divide-y divide-border text-sm">
      {/* Операции с клипом */}
      <section className="p-3 space-y-2">
        <h3 className="font-medium text-text truncate">{clip.name ?? 'Клип'}</h3>
        <div className="grid grid-cols-2 gap-2">
          {!isAudio && (
            <Button size="sm" variant="subtle" onClick={() => dispatch({ type: 'split' })}>
              <Scissors className="w-4 h-4" /> Разрезать
            </Button>
          )}
          <Button size="sm" variant="subtle" onClick={() => dispatch({ type: 'duplicate', id: clip.id })}>
            <Copy className="w-4 h-4" /> Дублировать
          </Button>
          {!isAudio && (
            <Button size="sm" variant="subtle" onClick={() => dispatch({ type: 'detachAudio', id: clip.id })}>
              <Volume2 className="w-4 h-4" /> Отделить звук
            </Button>
          )}
          <Button size="sm" variant="danger" onClick={() => dispatch({ type: 'delete', id: clip.id })}>
            <Trash2 className="w-4 h-4" /> Удалить
          </Button>
        </div>
        {!isAudio && (
          <div className="flex gap-2 pt-1">
            <Button size="sm" variant="ghost" onClick={() => dispatch({ type: 'reorder', id: clip.id, dir: -1 })}>
              ← Левее
            </Button>
            <Button size="sm" variant="ghost" onClick={() => dispatch({ type: 'reorder', id: clip.id, dir: 1 })}>
              Правее →
            </Button>
          </div>
        )}
      </section>

      {/* Обрезка */}
      <section className="p-3 space-y-2">
        <h4 className="font-medium text-text flex items-center gap-1.5">
          <Scissors className="w-4 h-4" /> Обрезка
        </h4>
        <NumberRow
          label="Начало (с)"
          value={clip.trimStart}
          min={0}
          max={clip.trimEnd - 0.1}
          step={0.1}
          onChange={(v) => dispatch({ type: 'setTrim', id: clip.id, trimStart: v, trimEnd: clip.trimEnd })}
        />
        <NumberRow
          label="Конец (с)"
          value={clip.trimEnd}
          min={clip.trimStart + 0.1}
          max={clip.sourceDuration}
          step={0.1}
          onChange={(v) => dispatch({ type: 'setTrim', id: clip.id, trimStart: clip.trimStart, trimEnd: v })}
        />
        <p className="text-xs text-muted">Длительность: {(clip.trimEnd - clip.trimStart).toFixed(1)} с</p>
      </section>

      {/* Цветокоррекция — только для видео */}
      {!isAudio && (
        <section className="p-3 space-y-3">
          <div className="flex items-center justify-between">
            <h4 className="font-medium text-text flex items-center gap-1.5">
              <Wand2 className="w-4 h-4" /> Цвет
            </h4>
            <button
              className="text-xs text-muted hover:text-text flex items-center gap-1"
              onClick={() => dispatch({ type: 'resetColor', id: clip.id })}
            >
              <RotateCcw className="w-3 h-3" /> Сбросить всё
            </button>
          </div>

          <div className="flex flex-wrap gap-1.5">
            {COLOR_PRESETS.map((p) => (
              <button
                key={p.name}
                onClick={() => dispatch({ type: 'setColor', id: clip.id, color: { ...NEUTRAL_COLOR, ...p.color } })}
                className="px-2 py-1 rounded-md border border-border bg-surface text-xs hover:bg-elevated"
              >
                {p.name}
              </button>
            ))}
          </div>

          <div className="space-y-2.5">
            {COLOR_PARAMS.map((p) => (
              <SliderRow
                key={p.key}
                label={p.label}
                value={color[p.key]}
                min={p.min}
                max={p.max}
                step={p.step}
                neutral={NEUTRAL_COLOR[p.key]}
                onChange={(v) => dispatch({ type: 'updateColor', id: clip.id, patch: { [p.key]: v } as Partial<ClipColor> })}
                onReset={() => dispatch({ type: 'updateColor', id: clip.id, patch: { [p.key]: NEUTRAL_COLOR[p.key] } as Partial<ClipColor> })}
              />
            ))}
          </div>
        </section>
      )}

      {/* Звук */}
      <section className="p-3 space-y-3">
        <h4 className="font-medium text-text flex items-center gap-1.5">
          {audio.muted ? <VolumeX className="w-4 h-4" /> : <Volume2 className="w-4 h-4" />} Звук
        </h4>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={!!audio.muted}
            onChange={(e) => dispatch({ type: 'updateAudio', id: clip.id, patch: { muted: e.target.checked } })}
          />
          Без звука (mute)
        </label>
        <SliderRow
          label="Громкость"
          value={audio.volume}
          min={0}
          max={2}
          step={0.01}
          neutral={1}
          disabled={audio.muted}
          onChange={(v) => dispatch({ type: 'updateAudio', id: clip.id, patch: { volume: v } })}
          onReset={() => dispatch({ type: 'updateAudio', id: clip.id, patch: { volume: 1 } })}
        />
        <SliderRow
          label="Fade In (с)"
          value={audio.fadeIn}
          min={0}
          max={5}
          step={0.1}
          neutral={0}
          onChange={(v) => dispatch({ type: 'updateAudio', id: clip.id, patch: { fadeIn: v } })}
          onReset={() => dispatch({ type: 'updateAudio', id: clip.id, patch: { fadeIn: 0 } })}
        />
        <SliderRow
          label="Fade Out (с)"
          value={audio.fadeOut}
          min={0}
          max={5}
          step={0.1}
          neutral={0}
          onChange={(v) => dispatch({ type: 'updateAudio', id: clip.id, patch: { fadeOut: v } })}
          onReset={() => dispatch({ type: 'updateAudio', id: clip.id, patch: { fadeOut: 0 } })}
        />
      </section>
    </div>
  )
}

function SliderRow({
  label,
  value,
  min,
  max,
  step,
  neutral,
  disabled,
  onChange,
  onReset,
}: {
  label: string
  value: number
  min: number
  max: number
  step: number
  neutral: number
  disabled?: boolean
  onChange: (v: number) => void
  onReset: () => void
}) {
  const changed = Math.abs(value - neutral) > 1e-6
  return (
    <div className={cn('space-y-1', disabled && 'opacity-50')}>
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted">{label}</span>
        <span className="flex items-center gap-1.5">
          <span className="tabular-nums text-text">{value.toFixed(2)}</span>
          {changed && (
            <button onClick={onReset} title="Сбросить параметр" className="text-muted hover:text-text">
              <RotateCcw className="w-3 h-3" />
            </button>
          )}
        </span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        className="w-full accent-brand"
      />
    </div>
  )
}

function NumberRow({
  label,
  value,
  min,
  max,
  step,
  onChange,
}: {
  label: string
  value: number
  min: number
  max: number
  step: number
  onChange: (v: number) => void
}) {
  return (
    <label className="flex items-center justify-between gap-2 text-xs">
      <span className="text-muted">{label}</span>
      <input
        type="number"
        value={Number(value.toFixed(2))}
        min={min}
        max={max}
        step={step}
        onChange={(e) => {
          const v = parseFloat(e.target.value)
          if (!Number.isNaN(v)) onChange(Math.max(min, Math.min(max, v)))
        }}
        className="w-24 h-8 rounded-md bg-surface border border-border px-2 text-text tabular-nums"
      />
    </label>
  )
}
