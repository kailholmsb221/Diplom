import { useCallback, useReducer } from 'react'
import type { ClipAudio, ClipColor, Timeline, TimelineClip } from '@/shared/types'
import { NEUTRAL_COLOR, withDefaultAudio, withNeutralColor } from './editorDefaults'

let idCounter = 0
function newId(prefix: string): string {
  idCounter += 1
  return `${prefix}-${Date.now().toString(36)}-${idCounter}`
}

export function clipDuration(c: TimelineClip): number {
  const d = c.trimEnd - c.trimStart
  if (Number.isFinite(d) && d > 0) return Math.max(0.05, d)
  // Данные клипа повреждены (NaN/undefined в trim) — НЕ схлопываем клип, а берём
  // длину источника; если и она неизвестна, временно 10 c (реальную подтянет probe).
  return Number.isFinite(c.sourceDuration) && c.sourceDuration > 0 ? c.sourceDuration : 10
}

function videoTrack(t: Timeline) {
  return t.tracks.find((tr) => tr.kind === 'video')
}
function audioTrack(t: Timeline) {
  return t.tracks.find((tr) => tr.kind === 'audio')
}

/** Раскладывает клипы видеодорожки последовательно (без зазоров) и пересчитывает длительность. */
function relayout(t: Timeline): Timeline {
  const tracks = t.tracks.map((tr) => {
    if (tr.kind !== 'video') return tr
    let cursor = 0
    const clips = tr.clips.map((c) => {
      const placed = { ...c, start: cursor }
      cursor += clipDuration(c)
      return placed
    })
    return { ...tr, clips }
  })
  let duration = 0
  for (const tr of tracks) {
    for (const c of tr.clips) duration = Math.max(duration, c.start + clipDuration(c))
  }
  return { ...t, tracks, duration: Math.max(0.1, duration) }
}

interface EngineState {
  timeline: Timeline
  past: Timeline[]
  future: Timeline[]
  selectedClipId: string | null
  playhead: number
  pxPerSec: number
  dirty: boolean
}

export type EngineAction =
  | { type: 'reset'; timeline: Timeline }
  | { type: 'select'; id: string | null }
  | { type: 'setPlayhead'; time: number }
  | { type: 'setZoom'; pxPerSec: number }
  | { type: 'undo' }
  | { type: 'redo' }
  | { type: 'markSaved' }
  | { type: 'split' }
  | { type: 'setTrim'; id: string; trimStart: number; trimEnd: number }
  | { type: 'reorder'; id: string; dir: -1 | 1 }
  | { type: 'delete'; id: string }
  | { type: 'duplicate'; id: string }
  | { type: 'addVideoClip'; clip: Omit<TimelineClip, 'id' | 'start' | 'color' | 'audio'> }
  | { type: 'addAudioClip'; clip: Omit<TimelineClip, 'id' | 'color' | 'audio'>; audio?: Partial<ClipAudio> }
  | { type: 'detachAudio'; id: string }
  | { type: 'updateColor'; id: string; patch: Partial<ClipColor> }
  | { type: 'setColor'; id: string; color: ClipColor }
  | { type: 'resetColor'; id: string }
  | { type: 'updateAudio'; id: string; patch: Partial<ClipAudio> }
  | { type: 'moveAudioClip'; id: string; start: number }
  | { type: 'repairClip'; id: string; sourceDuration: number; trimEnd: number }

/** Применяет мутацию к таймлайну с записью в историю (для undo/redo). */
function commit(state: EngineState, next: Timeline, selectId?: string | null): EngineState {
  return {
    ...state,
    past: [...state.past, state.timeline].slice(-50),
    future: [],
    timeline: relayout(next),
    dirty: true,
    selectedClipId: selectId === undefined ? state.selectedClipId : selectId,
  }
}

function mapClips(t: Timeline, fn: (c: TimelineClip, kind: string) => TimelineClip): Timeline {
  return { ...t, tracks: t.tracks.map((tr) => ({ ...tr, clips: tr.clips.map((c) => fn(c, tr.kind)) })) }
}

function findClip(t: Timeline, id: string): TimelineClip | undefined {
  for (const tr of t.tracks) {
    const c = tr.clips.find((x) => x.id === id)
    if (c) return c
  }
  return undefined
}

function reducer(state: EngineState, action: EngineAction): EngineState {
  switch (action.type) {
    case 'reset':
      return {
        timeline: relayout(action.timeline),
        past: [],
        future: [],
        selectedClipId: findClip(action.timeline, '')?.id ?? firstVideoClipId(action.timeline),
        playhead: 0,
        pxPerSec: state.pxPerSec || 60,
        dirty: false,
      }
    case 'select':
      return { ...state, selectedClipId: action.id }
    case 'setPlayhead':
      return { ...state, playhead: Math.max(0, Math.min(action.time, state.timeline.duration)) }
    case 'setZoom':
      return { ...state, pxPerSec: Math.max(10, Math.min(400, action.pxPerSec)) }
    case 'markSaved':
      return { ...state, dirty: false }
    case 'undo': {
      if (state.past.length === 0) return state
      const prev = state.past[state.past.length - 1]
      return {
        ...state,
        timeline: prev,
        past: state.past.slice(0, -1),
        future: [state.timeline, ...state.future].slice(0, 50),
        dirty: true,
      }
    }
    case 'redo': {
      if (state.future.length === 0) return state
      const next = state.future[0]
      return {
        ...state,
        timeline: next,
        past: [...state.past, state.timeline].slice(-50),
        future: state.future.slice(1),
        dirty: true,
      }
    }
    case 'split': {
      const vt = videoTrack(state.timeline)
      if (!vt) return state
      const ph = state.playhead
      const target = vt.clips.find((c) => ph > c.start + 0.05 && ph < c.start + clipDuration(c) - 0.05)
      if (!target) return state
      const offset = ph - target.start // секунды от начала клипа
      const cutSource = target.trimStart + offset
      const left: TimelineClip = { ...target, id: newId('clip'), trimEnd: cutSource }
      const right: TimelineClip = { ...target, id: newId('clip'), trimStart: cutSource }
      const clips: TimelineClip[] = []
      for (const c of vt.clips) {
        if (c.id === target.id) clips.push(left, right)
        else clips.push(c)
      }
      const next = { ...state.timeline, tracks: state.timeline.tracks.map((tr) => (tr.kind === 'video' ? { ...tr, clips } : tr)) }
      return commit(state, next, right.id)
    }
    case 'setTrim': {
      const next = mapClips(state.timeline, (c) => {
        if (c.id !== action.id) return c
        const ts = Math.max(0, Math.min(action.trimStart, c.sourceDuration - 0.05))
        const te = Math.max(ts + 0.05, Math.min(action.trimEnd, c.sourceDuration))
        return { ...c, trimStart: ts, trimEnd: te }
      })
      return commit(state, next)
    }
    case 'reorder': {
      const vt = videoTrack(state.timeline)
      if (!vt) return state
      const i = vt.clips.findIndex((c) => c.id === action.id)
      const j = i + action.dir
      if (i < 0 || j < 0 || j >= vt.clips.length) return state
      const clips = [...vt.clips]
      ;[clips[i], clips[j]] = [clips[j], clips[i]]
      const next = { ...state.timeline, tracks: state.timeline.tracks.map((tr) => (tr.kind === 'video' ? { ...tr, clips } : tr)) }
      return commit(state, next)
    }
    case 'delete': {
      const next = { ...state.timeline, tracks: state.timeline.tracks.map((tr) => ({ ...tr, clips: tr.clips.filter((c) => c.id !== action.id) })) }
      return commit(state, next, null)
    }
    case 'duplicate': {
      const src = findClip(state.timeline, action.id)
      if (!src) return state
      const copy = { ...src, id: newId('clip') }
      const next = {
        ...state.timeline,
        tracks: state.timeline.tracks.map((tr) => {
          if (!tr.clips.some((c) => c.id === action.id)) return tr
          const clips: TimelineClip[] = []
          for (const c of tr.clips) {
            clips.push(c)
            if (c.id === action.id) clips.push(copy)
          }
          return { ...tr, clips }
        }),
      }
      return commit(state, next, copy.id)
    }
    case 'addVideoClip': {
      const clip: TimelineClip = { ...action.clip, id: newId('clip'), start: 0, color: { ...NEUTRAL_COLOR }, audio: withDefaultAudio() }
      const next = { ...state.timeline, tracks: state.timeline.tracks.map((tr) => (tr.kind === 'video' ? { ...tr, clips: [...tr.clips, clip] } : tr)) }
      return commit(state, next, clip.id)
    }
    case 'addAudioClip': {
      const clip: TimelineClip = { ...action.clip, id: newId('aclip'), audio: withDefaultAudio(action.audio) }
      let next = { ...state.timeline }
      if (!audioTrack(next)) next = { ...next, tracks: [...next.tracks, { id: newId('audio'), kind: 'audio', clips: [] }] }
      next = { ...next, tracks: next.tracks.map((tr) => (tr.kind === 'audio' ? { ...tr, clips: [...tr.clips, clip] } : tr)) }
      return commit(state, next, clip.id)
    }
    case 'detachAudio': {
      const src = findClip(state.timeline, action.id)
      if (!src) return state
      // 1) глушим звук исходного видеоклипа; 2) добавляем аудиоклип на аудиодорожку.
      let next = mapClips(state.timeline, (c) => (c.id === action.id ? { ...c, audio: withDefaultAudio({ ...c.audio, muted: true }) } : c))
      const aclip: TimelineClip = {
        id: newId('aclip'),
        sourceUrl: src.sourceUrl,
        name: (src.name ?? 'Аудио') + ' (звук)',
        sourceDuration: src.sourceDuration,
        trimStart: src.trimStart,
        trimEnd: src.trimEnd,
        start: src.start,
        audio: withDefaultAudio(),
      }
      if (!audioTrack(next)) next = { ...next, tracks: [...next.tracks, { id: newId('audio'), kind: 'audio', clips: [] }] }
      next = { ...next, tracks: next.tracks.map((tr) => (tr.kind === 'audio' ? { ...tr, clips: [...tr.clips, aclip] } : tr)) }
      return commit(state, next, aclip.id)
    }
    case 'updateColor': {
      const next = mapClips(state.timeline, (c) => (c.id === action.id ? { ...c, color: withNeutralColor({ ...c.color, ...action.patch }) } : c))
      return commit(state, next)
    }
    case 'setColor': {
      const next = mapClips(state.timeline, (c) => (c.id === action.id ? { ...c, color: action.color } : c))
      return commit(state, next)
    }
    case 'resetColor': {
      const next = mapClips(state.timeline, (c) => (c.id === action.id ? { ...c, color: { ...NEUTRAL_COLOR } } : c))
      return commit(state, next)
    }
    case 'updateAudio': {
      const next = mapClips(state.timeline, (c) => (c.id === action.id ? { ...c, audio: withDefaultAudio({ ...c.audio, ...action.patch }) } : c))
      return commit(state, next)
    }
    case 'moveAudioClip': {
      const next = mapClips(state.timeline, (c, kind) => (c.id === action.id && kind === 'audio' ? { ...c, start: Math.max(0, action.start) } : c))
      return commit(state, next)
    }
    case 'repairClip': {
      // Восстановление длительности клипа с битыми данными (probe нашёл реальную
      // длину). Не пишем в историю undo, но помечаем dirty — чтобы починка сохранилась.
      const next = mapClips(state.timeline, (c) => {
        if (c.id !== action.id) return c
        const ts = Number.isFinite(c.trimStart) && c.trimStart >= 0 ? c.trimStart : 0
        return { ...c, sourceDuration: action.sourceDuration, trimStart: ts, trimEnd: action.trimEnd }
      })
      return { ...state, timeline: relayout(next), dirty: true }
    }
    default:
      return state
  }
}

function firstVideoClipId(t: Timeline): string | null {
  const vt = videoTrack(t)
  return vt && vt.clips.length > 0 ? vt.clips[0].id : null
}

export function useEditorEngine(initial: Timeline) {
  const [state, dispatch] = useReducer(reducer, undefined, () => ({
    timeline: relayout(initial),
    past: [],
    future: [],
    selectedClipId: firstVideoClipId(initial),
    playhead: 0,
    pxPerSec: 60,
    dirty: false,
  }))

  const selectedClip = state.selectedClipId ? findClip(state.timeline, state.selectedClipId) ?? null : null

  const canUndo = state.past.length > 0
  const canRedo = state.future.length > 0

  const reset = useCallback((timeline: Timeline) => dispatch({ type: 'reset', timeline }), [])

  return { state, dispatch, selectedClip, canUndo, canRedo, reset, findClip: (id: string) => findClip(state.timeline, id) }
}

export { findClip, videoTrack, audioTrack }
