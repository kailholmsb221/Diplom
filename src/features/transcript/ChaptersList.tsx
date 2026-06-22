import { Clock } from 'lucide-react'
import { formatDuration } from '@/shared/lib/format'
import type { Chapter } from '@/shared/types'

interface ChaptersListProps {
  chapters: Chapter[]
  onSeek: (seconds: number) => void
}

/**
 * Кликабельные таймкоды в стиле YouTube под видео.
 * Скрываем компонент, если глав нет — backend генерирует их автоматически,
 * и до завершения транскрипции список пустой.
 */
export function ChaptersList({ chapters, onSeek }: ChaptersListProps) {
  if (!chapters?.length) return null
  return (
    <section className="mt-4">
      <h3 className="font-semibold mb-2 flex items-center gap-2">
        <Clock className="w-4 h-4" />
        Таймкоды
      </h3>
      <ul className="space-y-1">
        {chapters.map((c, i) => (
          <li key={i}>
            <button
              onClick={() => onSeek(c.start)}
              className="w-full text-left flex gap-3 px-2 py-1 rounded hover:bg-elevated transition-colors"
            >
              <span className="font-mono text-sm text-brand min-w-[3rem]">
                {formatDuration(c.start)}
              </span>
              <span className="text-sm">{c.title}</span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}
