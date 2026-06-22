import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Languages, Loader2, AlertCircle, RefreshCw } from 'lucide-react'
import {
  fetchTranscript,
  rerunTranscription,
  rerunTranslation,
} from '@/api/transcripts'
import { Button } from '@/shared/ui/Button'
import { formatDuration } from '@/shared/lib/format'
import { toast } from '@/shared/ui/toast'
import { useIsAdmin } from '@/stores/authStore'
import type { TranscriptLanguage } from '@/shared/types'

const LANG_LABELS: Record<TranscriptLanguage, string> = {
  ru: 'Русский',
  kk: 'Қазақша',
  en: 'English',
}

interface TranscriptPanelProps {
  videoId: string
  /** Колбэк для перехода видео к моменту времени (seek в плеере). */
  onSeek: (seconds: number) => void
}

export function TranscriptPanel({ videoId, onSeek }: TranscriptPanelProps) {
  const qc = useQueryClient()
  const isAdmin = useIsAdmin()
  const [lang, setLang] = useState<TranscriptLanguage>('ru')

  const { data, isLoading } = useQuery({
    queryKey: ['transcript', videoId, lang],
    queryFn: () => fetchTranscript(videoId, lang),
    // Если процесс идёт — опрашиваем каждые 4 секунды, чтобы UI обновлялся.
    refetchInterval: (q) => {
      const s = q.state.data?.status
      return s === 'PROCESSING' || s === 'TRANSLATING' ? 4_000 : false
    },
  })

  // Сброс на оригинальный язык, когда он определён.
  useEffect(() => {
    if (data?.originalLanguage && !sessionStorage.getItem(`tr:${videoId}`)) {
      setLang(data.originalLanguage as TranscriptLanguage)
      sessionStorage.setItem(`tr:${videoId}`, '1')
    }
  }, [data?.originalLanguage, videoId])

  const restart = useMutation({
    mutationFn: () => rerunTranscription(videoId),
    onSuccess: () => {
      toast('Расшифровка запущена заново', 'success')
      qc.invalidateQueries({ queryKey: ['transcript', videoId] })
      qc.invalidateQueries({ queryKey: ['subtitles', videoId] })
    },
    onError: () => toast('Не удалось запустить расшифровку', 'error'),
  })

  const retranslate = useMutation({
    mutationFn: () => rerunTranslation(videoId),
    onSuccess: () => {
      toast('Перевод запущен', 'success')
      qc.invalidateQueries({ queryKey: ['transcript', videoId] })
      qc.invalidateQueries({ queryKey: ['subtitles', videoId] })
    },
    onError: () => toast('Не удалось запустить перевод', 'error'),
  })

  return (
    <section className="bg-surface border border-border rounded-2xl p-4">
      <div className="flex items-center gap-3 flex-wrap mb-3">
        <h3 className="font-semibold flex items-center gap-2">
          <Languages className="w-5 h-5" />
          Расшифровка
        </h3>
        <div className="flex gap-1 bg-elevated rounded-lg p-1 ml-auto">
          {(Object.keys(LANG_LABELS) as TranscriptLanguage[]).map((l) => (
            <button
              key={l}
              onClick={() => setLang(l)}
              className={`px-3 h-7 text-xs rounded-md transition-colors ${
                lang === l ? 'bg-bg shadow-sm font-semibold' : 'text-muted hover:text-text'
              }`}
            >
              {LANG_LABELS[l]}
            </button>
          ))}
        </div>
      </div>

      {isLoading && <BodyMessage icon="spin">Загружаем расшифровку…</BodyMessage>}

      {!isLoading && data && (
        <>
          {data.status === 'NOT_STARTED' && (
            <BodyMessage>
              Расшифровка ещё не запускалась.
              {isAdmin && (
                <Button size="sm" className="mt-3" onClick={() => restart.mutate()}>
                  <RefreshCw className="w-4 h-4" /> Запустить
                </Button>
              )}
            </BodyMessage>
          )}

          {data.status === 'PROCESSING' && (
            <BodyMessage icon="spin">Распознаём речь… это может занять несколько минут.</BodyMessage>
          )}

          {data.status === 'TRANSLATING' && (
            <BodyMessage icon="spin">
              Перевод на {LANG_LABELS[lang]} создаётся…
            </BodyMessage>
          )}

          {data.status === 'FAILED' && (
            <BodyMessage icon="error">
              <div>Не удалось создать расшифровку{data.error ? `: ${data.error}` : '.'}</div>
              <div className="flex gap-2 mt-3 flex-wrap">
                {lang !== data.originalLanguage && data.originalLanguage && (
                  <Button size="sm" onClick={() => retranslate.mutate()}>
                    <RefreshCw className="w-4 h-4" /> Создать перевод
                  </Button>
                )}
                {isAdmin && (
                  <Button size="sm" variant="secondary" onClick={() => restart.mutate()}>
                    Перезапустить распознавание
                  </Button>
                )}
              </div>
            </BodyMessage>
          )}

          {data.status === 'COMPLETED' && (
            <SegmentList segments={data.segments} onSeek={onSeek} />
          )}
        </>
      )}
    </section>
  )
}

function BodyMessage({
  children,
  icon,
}: {
  children: React.ReactNode
  icon?: 'spin' | 'error'
}) {
  return (
    <div className="text-sm text-muted py-6 flex flex-col items-center gap-3 text-center">
      {icon === 'spin' && <Loader2 className="w-5 h-5 animate-spin text-brand" />}
      {icon === 'error' && <AlertCircle className="w-5 h-5 text-danger" />}
      <div>{children}</div>
    </div>
  )
}

function SegmentList({
  segments,
  onSeek,
}: {
  segments: { start: number; end: number; text: string }[]
  onSeek: (seconds: number) => void
}) {
  if (!segments.length) {
    return <div className="text-sm text-muted py-6 text-center">Сегментов нет.</div>
  }
  return (
    <ul className="max-h-96 overflow-y-auto space-y-1 pr-2">
      {segments.map((s, i) => (
        <li key={i}>
          <button
            onClick={() => onSeek(s.start)}
            className="w-full text-left flex gap-3 px-2 py-1.5 rounded-md hover:bg-elevated transition-colors"
          >
            <span className="font-mono text-xs text-brand whitespace-nowrap pt-0.5 min-w-[3rem]">
              {formatDuration(s.start)}
            </span>
            <span className="text-sm flex-1">{s.text}</span>
          </button>
        </li>
      ))}
    </ul>
  )
}
