import type { ClipAudio, ClipColor } from '@/shared/types'

/** Нейтральная цветокоррекция — картинка без изменений. */
export const NEUTRAL_COLOR: ClipColor = {
  brightness: 0,
  contrast: 1,
  saturation: 1,
  exposure: 1,
  temperature: 0,
  hue: 0,
  highlights: 0,
  shadows: 0,
  sharpness: 0,
  opacity: 1,
}

export const DEFAULT_AUDIO: ClipAudio = {
  muted: false,
  volume: 1,
  fadeIn: 0,
  fadeOut: 0,
}

export interface ColorParamMeta {
  key: keyof ClipColor
  label: string
  min: number
  max: number
  step: number
}

/** Описание ползунков цветокоррекции (диапазоны согласованы с backend/render.py). */
export const COLOR_PARAMS: ColorParamMeta[] = [
  { key: 'brightness', label: 'Яркость', min: -1, max: 1, step: 0.01 },
  { key: 'contrast', label: 'Контраст', min: 0, max: 2, step: 0.01 },
  { key: 'saturation', label: 'Насыщенность', min: 0, max: 3, step: 0.01 },
  { key: 'exposure', label: 'Экспозиция', min: 0.1, max: 3, step: 0.01 },
  { key: 'temperature', label: 'Температура', min: -100, max: 100, step: 1 },
  { key: 'hue', label: 'Оттенок', min: -180, max: 180, step: 1 },
  { key: 'highlights', label: 'Светлые участки', min: -1, max: 1, step: 0.01 },
  { key: 'shadows', label: 'Тени', min: -1, max: 1, step: 0.01 },
  { key: 'sharpness', label: 'Резкость', min: 0, max: 2, step: 0.01 },
  { key: 'opacity', label: 'Прозрачность', min: 0, max: 1, step: 0.01 },
]

export interface ColorPreset {
  name: string
  color: Partial<ClipColor>
}

/** Готовые фильтры. Применяются поверх нейтральной базы. */
export const COLOR_PRESETS: ColorPreset[] = [
  { name: 'Оригинал', color: {} },
  { name: 'Тёплый', color: { temperature: 45, saturation: 1.15, brightness: 0.05 } },
  { name: 'Холодный', color: { temperature: -45, saturation: 1.1, contrast: 1.05 } },
  { name: 'Ч/Б', color: { saturation: 0, contrast: 1.1 } },
  { name: 'Винтаж', color: { temperature: 30, saturation: 0.8, contrast: 0.9, exposure: 1.05 } },
  { name: 'Яркий', color: { saturation: 1.4, contrast: 1.2, brightness: 0.08 } },
  { name: 'Кино', color: { contrast: 1.25, saturation: 1.1, shadows: -0.2, highlights: 0.1 } },
]

/** Преобразует цветокоррекцию в CSS-фильтр для мгновенного превью (без перекодирования). */
export function clipColorToCss(c?: ClipColor): { filter: string; opacity: number } {
  if (!c) return { filter: 'none', opacity: 1 }
  const brightness = Math.max(0, (1 + c.brightness) * (c.exposure || 1) + c.shadows * 0.15)
  const contrast = Math.max(0, c.contrast + c.highlights * 0.2)
  const saturate = Math.max(0, c.saturation)
  const parts = [
    `brightness(${brightness.toFixed(3)})`,
    `contrast(${contrast.toFixed(3)})`,
    `saturate(${saturate.toFixed(3)})`,
    `hue-rotate(${c.hue.toFixed(1)}deg)`,
  ]
  if (c.temperature > 0) parts.push(`sepia(${Math.min(1, c.temperature / 200).toFixed(3)})`)
  return { filter: parts.join(' '), opacity: c.opacity ?? 1 }
}

export function withNeutralColor(partial?: Partial<ClipColor>): ClipColor {
  return { ...NEUTRAL_COLOR, ...(partial ?? {}) }
}

export function withDefaultAudio(partial?: Partial<ClipAudio>): ClipAudio {
  return { ...DEFAULT_AUDIO, ...(partial ?? {}) }
}
