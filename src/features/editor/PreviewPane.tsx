import { useEffect, useMemo, useRef } from 'react'
import { Pause, Play, SkipBack } from 'lucide-react'
import { Button } from '@/shared/ui/Button'
import { formatDuration } from '@/shared/lib/format'
import type { Timeline, TimelineClip } from '@/shared/types'
import { clipColorToCss } from './editorDefaults'
import { audioTrack, clipDuration, videoTrack } from './useEditorEngine'

interface PreviewPaneProps {
  timeline: Timeline
  playhead: number
  playing: boolean
  onTime: (t: number) => void
  onTogglePlay: () => void
}

/** Возвращает активный видеоклип под playhead и время внутри исходника. */
function activeAt(timeline: Timeline, ph: number): { clip: TimelineClip; sourceTime: number } | null {
  const vt = videoTrack(timeline)
  if (!vt) return null
  for (const c of vt.clips) {
    const dur = clipDuration(c)
    if (ph >= c.start - 0.001 && ph < c.start + dur) {
      return { clip: c, sourceTime: c.trimStart + (ph - c.start) }
    }
  }
  const last = vt.clips[vt.clips.length - 1]
  if (last && ph >= last.start + clipDuration(last)) {
    return { clip: last, sourceTime: last.trimEnd }
  }
  return vt.clips[0] ? { clip: vt.clips[0], sourceTime: vt.clips[0].trimStart } : null
}

export function PreviewPane({ timeline, playhead, playing, onTime, onTogglePlay }: PreviewPaneProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const active = useMemo(() => activeAt(timeline, playhead), [timeline, playhead])
  const activeId = active?.clip.id ?? null
  const loadedSrcRef = useRef<string>('')

  const css = clipColorToCss(active?.clip.color)

  // Смена исходника активного клипа.
  useEffect(() => {
    const v = videoRef.current
    if (!v || !active) return
    if (loadedSrcRef.current !== active.clip.sourceUrl) {
      loadedSrcRef.current = active.clip.sourceUrl
      v.src = active.clip.sourceUrl
      v.load()
      const seek = () => {
        v.currentTime = active.sourceTime
        v.removeEventListener('loadedmetadata', seek)
      }
      v.addEventListener('loadedmetadata', seek)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeId])

  // Громкость / mute активного клипа.
  useEffect(() => {
    const v = videoRef.current
    if (!v || !active) return
    const a = active.clip.audio
    v.muted = !!a?.muted
    v.volume = Math.max(0, Math.min(1, a?.volume ?? 1))
  }, [active])

  // Скраб: когда не играем — двигаем видео за playhead.
  useEffect(() => {
    const v = videoRef.current
    if (!v || !active || playing) return
    if (loadedSrcRef.current !== active.clip.sourceUrl) return
    if (Math.abs(v.currentTime - active.sourceTime) > 0.08) {
      try {
        v.currentTime = active.sourceTime
      } catch {
        /* not seekable yet */
      }
    }
  }, [playhead, playing, active])

  // Play / pause.
  useEffect(() => {
    const v = videoRef.current
    if (!v) return
    if (playing) v.play().catch(() => undefined)
    else v.pause()
  }, [playing, activeId])

  // Видео — часы во время воспроизведения: маппим в время таймлайна и листаем клипы.
  useEffect(() => {
    const v = videoRef.current
    if (!v) return
    const onTimeUpdate = () => {
      if (!playing || !active) return
      const tl = active.clip.start + (v.currentTime - active.clip.trimStart)
      if (v.currentTime >= active.clip.trimEnd - 0.03) {
        const next = active.clip.start + clipDuration(active.clip)
        if (next >= timeline.duration - 0.05) {
          onTime(0)
          onTogglePlay() // стоп в конце
        } else {
          onTime(next + 0.01)
        }
        return
      }
      onTime(tl)
    }
    v.addEventListener('timeupdate', onTimeUpdate)
    return () => v.removeEventListener('timeupdate', onTimeUpdate)
  }, [playing, active, timeline.duration, onTime, onTogglePlay])

  const audioClips = audioTrack(timeline)?.clips ?? []

  return (
    <div className="flex flex-col gap-3">
      <div className="relative bg-black rounded-xl overflow-hidden aspect-video grid place-items-center">
        {active ? (
          <video
            ref={videoRef}
            className="w-full h-full object-contain"
            style={{ filter: css.filter, opacity: css.opacity }}
            playsInline
            onClick={onTogglePlay}
          />
        ) : (
          <div className="text-muted text-sm">Нет клипов на таймлайне</div>
        )}
      </div>

      {/* Аудиодорожка: отделённый звук, музыка и озвучка. Скрытые <audio>,
          синхронизированные с playhead — играют параллельно с видео. */}
      {audioClips.map((clip) => (
        <AudioTrackClip key={clip.id} clip={clip} playhead={playhead} playing={playing} />
      ))}
      <div className="flex items-center gap-3">
        <Button size="icon" variant="secondary" onClick={() => onTime(0)} title="В начало">
          <SkipBack className="w-4 h-4" />
        </Button>
        <Button size="icon" variant="primary" onClick={onTogglePlay} title="Пробел">
          {playing ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4" />}
        </Button>
        <span className="text-sm text-muted tabular-nums">
          {formatDuration(Math.round(playhead))} / {formatDuration(Math.round(timeline.duration))}
        </span>
      </div>
    </div>
  )
}

/**
 * Один аудиоклип на аудиодорожке (отделённый звук / музыка / озвучка).
 * Воспроизводится синхронно с playhead: пока playhead внутри [start; start+dur]
 * и идёт проигрывание — звучит; иначе на паузе. При скрабе позиция держится
 * в кадре. Громкость учитывает mute и затухания (fadeIn/fadeOut).
 */
function AudioTrackClip({ clip, playhead, playing }: { clip: TimelineClip; playhead: number; playing: boolean }) {
  const ref = useRef<HTMLAudioElement>(null)
  const dur = clipDuration(clip)
  const within = playhead >= clip.start - 0.001 && playhead < clip.start + dur - 0.001
  const sourceTime = clip.trimStart + Math.max(0, playhead - clip.start)

  // Громкость с учётом mute и затуханий — зависит от позиции внутри клипа.
  useEffect(() => {
    const a = ref.current
    if (!a) return
    const base = clip.audio?.muted ? 0 : Math.max(0, Math.min(1, clip.audio?.volume ?? 1))
    const pos = sourceTime - clip.trimStart
    let f = 1
    const fin = clip.audio?.fadeIn ?? 0
    const fout = clip.audio?.fadeOut ?? 0
    if (fin > 0.01 && pos < fin) f = Math.max(0, pos / fin)
    if (fout > 0.01 && pos > dur - fout) f = Math.min(f, Math.max(0, (dur - pos) / fout))
    a.volume = Math.max(0, Math.min(1, base * f))
  }, [clip.audio, sourceTime, dur, clip.trimStart])

  // Воспроизведение / пауза + синхронизация по playhead.
  useEffect(() => {
    const a = ref.current
    if (!a) return
    if (within && playing) {
      if (a.paused) {
        try {
          a.currentTime = sourceTime
        } catch {
          /* not seekable yet */
        }
        a.play().catch(() => undefined)
      } else if (Math.abs(a.currentTime - sourceTime) > 0.35) {
        // Накопилась рассинхронизация — подтягиваем.
        try {
          a.currentTime = sourceTime
        } catch {
          /* not seekable yet */
        }
      }
    } else {
      if (!a.paused) a.pause()
      // Скраб внутри клипа на паузе: держим позицию в кадре (беззвучно).
      if (!playing && within && Math.abs(a.currentTime - sourceTime) > 0.08) {
        try {
          a.currentTime = sourceTime
        } catch {
          /* not seekable yet */
        }
      }
    }
  }, [within, playing, sourceTime])

  return <audio ref={ref} src={clip.sourceUrl} preload="auto" />
}
