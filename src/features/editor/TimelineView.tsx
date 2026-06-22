import { useEffect, useRef, useState, type Dispatch } from 'react'
import { Maximize2, Music2, Video as VideoIcon, ZoomIn, ZoomOut } from 'lucide-react'
import { cn } from '@/shared/lib/cn'
import type { Timeline, TimelineClip } from '@/shared/types'
import { clipDuration, type EngineAction } from './useEditorEngine'

const RULER_H = 28
const VIDEO_H = 64
const AUDIO_H = 56

interface TimelineViewProps {
  timeline: Timeline
  playhead: number
  pxPerSec: number
  selectedClipId: string | null
  dispatch: Dispatch<EngineAction>
  /** Вызывается в начале перемотки плейхеда — чтобы поставить воспроизведение на паузу. */
  onScrub?: () => void
}

interface TrimDrag {
  clipId: string
  edge: 'start' | 'end'
  startX: number
  origStart: number
  origEnd: number
  curStart: number
  curEnd: number
}

export function TimelineView({ timeline, playhead, pxPerSec, selectedClipId, dispatch, onScrub }: TimelineViewProps) {
  const laneRef = useRef<HTMLDivElement>(null)
  const draggingRef = useRef(false)
  const fittedRef = useRef(false)
  const [trim, setTrim] = useState<TrimDrag | null>(null)

  const duration = Number.isFinite(timeline.duration) && timeline.duration > 0 ? timeline.duration : 1
  const width = Math.max(400, duration * pxPerSec + 40)
  const sec = (t: number) => (Number.isFinite(t) ? t : 0) * pxPerSec

  /** Переводит X-координату курсора в секунды на таймлайне (с учётом прокрутки). */
  function timeFromClientX(clientX: number): number {
    const el = laneRef.current
    if (!el) return 0
    const rect = el.getBoundingClientRect()
    const x = clientX - rect.left + el.scrollLeft
    return Math.max(0, x / pxPerSec)
  }

  /** Перемотка плейхеда: пауза + прыжок в точку, затем непрерывно тянем за курсором. */
  function startScrub(e: React.MouseEvent) {
    e.preventDefault()
    onScrub?.() // остановить воспроизведение, чтобы видео не «убегало» от стрелки
    draggingRef.current = true // пока тянем — не даём авто-прокрутке мешать
    dispatch({ type: 'setPlayhead', time: timeFromClientX(e.clientX) })
    const onMove = (ev: MouseEvent) => {
      dispatch({ type: 'setPlayhead', time: timeFromClientX(ev.clientX) })
    }
    const onUp = () => {
      draggingRef.current = false
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  function setZoom(px: number) {
    dispatch({ type: 'setZoom', pxPerSec: px })
  }

  /** Подбирает масштаб так, чтобы весь ролик помещался в видимую ширину дорожки. */
  function fitToWindow() {
    const el = laneRef.current
    if (!el) return
    const avail = el.clientWidth - 24
    if (avail > 0 && duration > 0) setZoom(Math.max(10, Math.min(400, avail / duration)))
  }

  // Авто-вписывание при первой корректной длительности — чтобы клип был виден целиком.
  useEffect(() => {
    if (fittedRef.current) return
    const el = laneRef.current
    if (!el || el.clientWidth <= 0) return
    if (!Number.isFinite(timeline.duration) || timeline.duration <= 0) return
    fittedRef.current = true
    const avail = el.clientWidth - 24
    if (avail > 0) dispatch({ type: 'setZoom', pxPerSec: Math.max(10, Math.min(400, avail / timeline.duration)) })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timeline.duration])

  // Авто-прокрутка вслед за плейхедом (во время воспроизведения; не мешаем перетаскиванию).
  useEffect(() => {
    const el = laneRef.current
    if (!el || draggingRef.current) return
    const x = sec(playhead)
    const left = el.scrollLeft
    const right = left + el.clientWidth
    const margin = 48
    if (x > right - margin) el.scrollLeft = x - el.clientWidth + margin
    else if (x < left + margin) el.scrollLeft = Math.max(0, x - margin)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playhead, pxPerSec])

  function startTrim(e: React.MouseEvent, clip: TimelineClip, edge: 'start' | 'end') {
    e.stopPropagation()
    const drag: TrimDrag = {
      clipId: clip.id,
      edge,
      startX: e.clientX,
      origStart: clip.trimStart,
      origEnd: clip.trimEnd,
      curStart: clip.trimStart,
      curEnd: clip.trimEnd,
    }
    setTrim(drag)
    const onMove = (ev: MouseEvent) => {
      const deltaSec = (ev.clientX - drag.startX) / pxPerSec
      if (edge === 'start') drag.curStart = Math.max(0, Math.min(drag.origStart + deltaSec, drag.origEnd - 0.1))
      else drag.curEnd = Math.max(drag.origStart + 0.1, Math.min(drag.origEnd + deltaSec, clip.sourceDuration))
      setTrim({ ...drag })
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      dispatch({ type: 'setTrim', id: drag.clipId, trimStart: drag.curStart, trimEnd: drag.curEnd })
      setTrim(null)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  function startMoveAudio(e: React.MouseEvent, clip: TimelineClip) {
    e.stopPropagation()
    const startX = e.clientX
    const origStart = clip.start
    const onMove = (ev: MouseEvent) => {
      const deltaSec = (ev.clientX - startX) / pxPerSec
      dispatch({ type: 'moveAudioClip', id: clip.id, start: Math.max(0, origStart + deltaSec) })
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  const ticks: number[] = []
  const step = pxPerSec < 30 ? 10 : pxPerSec < 80 ? 5 : 1
  for (let t = 0; t <= duration + step; t += step) ticks.push(t)

  function visual(clip: TimelineClip) {
    if (trim && trim.clipId === clip.id) return { trimStart: trim.curStart, trimEnd: trim.curEnd }
    return { trimStart: clip.trimStart, trimEnd: clip.trimEnd }
  }

  return (
    <div className="flex flex-col gap-2">
      {/* Панель масштаба таймлайна. */}
      <div className="flex items-center gap-1.5 text-xs text-muted">
        <span className="mr-0.5">Масштаб:</span>
        <button onClick={() => setZoom(pxPerSec / 1.25)} className="p-1 rounded hover:bg-elevated text-text" title="Отдалить">
          <ZoomOut className="w-4 h-4" />
        </button>
        <button onClick={() => setZoom(pxPerSec * 1.25)} className="p-1 rounded hover:bg-elevated text-text" title="Приблизить">
          <ZoomIn className="w-4 h-4" />
        </button>
        <button
          onClick={fitToWindow}
          className="px-2 py-1 rounded hover:bg-elevated text-text flex items-center gap-1"
          title="Уместить весь ролик в окно"
        >
          <Maximize2 className="w-3.5 h-3.5" /> Уместить
        </button>
      </div>

      <div className="flex rounded-xl border border-border bg-surface overflow-hidden">
      {/* Левый жёлоб с названиями дорожек (не прокручивается). */}
      <div className="w-20 shrink-0 border-r border-border bg-elevated/40">
        <div style={{ height: RULER_H }} className="border-b border-border" />
        <div style={{ height: VIDEO_H }} className="border-b border-border flex items-center gap-1.5 px-2 text-xs text-muted">
          <VideoIcon className="w-3.5 h-3.5" />
          Видео
        </div>
        <div style={{ height: AUDIO_H }} className="flex items-center gap-1.5 px-2 text-xs text-muted">
          <Music2 className="w-3.5 h-3.5" />
          Аудио
        </div>
      </div>

      {/* Прокручиваемая область дорожек. */}
      <div ref={laneRef} className="flex-1 min-w-0 overflow-x-auto">
        <div style={{ width }} className="relative select-none">
          {/* Линейка */}
          <div
            style={{ height: RULER_H }}
            className="border-b border-border relative cursor-ew-resize bg-elevated"
            onMouseDown={startScrub}
          >
            {ticks.map((t) => (
              <div key={t} className="absolute top-0 h-full flex items-end pb-0.5" style={{ left: sec(t) }}>
                <div className="w-px h-2 bg-border" />
                <span className="text-[10px] text-muted ml-1">{formatTick(t)}</span>
              </div>
            ))}
          </div>

          {/* Видеодорожка */}
          <div style={{ height: VIDEO_H }} className="border-b border-border flex items-stretch gap-0.5 p-1">
            {videoClips(timeline).map((clip) => {
              const v = visual(clip)
              const vis = Math.max(0.1, clipDuration({ ...clip, trimStart: v.trimStart, trimEnd: v.trimEnd }))
              const w = sec(vis)
              return (
                <div
                  key={clip.id}
                  onMouseDown={() => dispatch({ type: 'select', id: clip.id })}
                  className={cn(
                    'relative h-full rounded-md overflow-hidden border text-xs text-white shrink-0 cursor-pointer',
                    'bg-gradient-to-br from-brand/80 to-brand/50',
                    selectedClipId === clip.id ? 'border-white ring-2 ring-brand' : 'border-black/20',
                  )}
                  style={{ width: w }}
                  title={clip.name}
                >
                  <span className="absolute left-2 top-1 truncate max-w-[88%] drop-shadow">{clip.name ?? 'Клип'}</span>
                  <span className="absolute left-2 bottom-1 text-[10px] opacity-80 tabular-nums">
                    {vis.toFixed(1)}с
                  </span>
                  <div
                    onMouseDown={(e) => startTrim(e, clip, 'start')}
                    className="absolute left-0 top-0 h-full w-2 bg-black/40 hover:bg-white/60 cursor-ew-resize"
                  />
                  <div
                    onMouseDown={(e) => startTrim(e, clip, 'end')}
                    className="absolute right-0 top-0 h-full w-2 bg-black/40 hover:bg-white/60 cursor-ew-resize"
                  />
                </div>
              )
            })}
            {videoClips(timeline).length === 0 && (
              <div className="text-xs text-muted px-2 self-center">Добавьте видео из медиатеки слева</div>
            )}
          </div>

          {/* Аудиодорожка */}
          <div style={{ height: AUDIO_H }} className="relative p-1">
            {audioClips(timeline).map((clip) => (
              <div
                key={clip.id}
                onMouseDown={(e) => {
                  dispatch({ type: 'select', id: clip.id })
                  startMoveAudio(e, clip)
                }}
                className={cn(
                  'absolute top-1 h-10 rounded-md border text-[11px] text-white overflow-hidden cursor-grab active:cursor-grabbing',
                  'bg-gradient-to-br from-emerald-500/80 to-emerald-600/60',
                  selectedClipId === clip.id ? 'border-white ring-2 ring-emerald-400' : 'border-black/20',
                )}
                style={{ left: sec(clip.start), width: sec(clipDuration(clip)) }}
                title={clip.name}
              >
                <span className="absolute left-2 top-1.5 truncate max-w-[88%] drop-shadow">{clip.name ?? 'Аудио'}</span>
              </div>
            ))}
            {audioClips(timeline).length === 0 && (
              <div className="text-xs text-muted px-2 self-center">Здесь появятся отделённый звук, музыка и озвучка</div>
            )}
          </div>

          {/* Playhead поверх всех дорожек. Линия не ловит мышь, а ручка-треугольник — ловит. */}
          <div className="pointer-events-none absolute top-0 bottom-0 w-px bg-red-500 z-10" style={{ left: sec(playhead) }}>
            <div
              onMouseDown={startScrub}
              className="pointer-events-auto absolute -top-1 -left-2 w-4 h-4 rotate-45 bg-red-500 cursor-ew-resize hover:bg-red-400"
              title="Перетащите, чтобы перемотать"
            />
          </div>
        </div>
      </div>
      </div>
    </div>
  )
}

function videoClips(t: Timeline): TimelineClip[] {
  return t.tracks.find((tr) => tr.kind === 'video')?.clips ?? []
}
function audioClips(t: Timeline): TimelineClip[] {
  return t.tracks.find((tr) => tr.kind === 'audio')?.clips ?? []
}

function formatTick(t: number): string {
  const m = Math.floor(t / 60)
  const s = Math.floor(t % 60)
  return m > 0 ? `${m}:${s.toString().padStart(2, '0')}` : `${s}s`
}
