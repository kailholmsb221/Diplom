import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { CheckCircle2, Loader2, X } from 'lucide-react'
import { Button } from '@/shared/ui/Button'
import { ApiError } from '@/api/client'
import { exportProject, getExportJob } from '@/api/editor'
import { toast } from '@/shared/ui/toast'
import type { Category, ExportSettings, RenderStatus, Visibility } from '@/shared/types'

interface ExportDialogProps {
  projectId: string
  defaultTitle: string
  onClose: () => void
  onBeforeExport: () => Promise<void> // сохранить проект перед экспортом
}

const RESOLUTIONS = [
  { label: '720p · 16:9 (1280×720)', width: 1280, height: 720 },
  { label: '1080p · 16:9 (1920×1080)', width: 1920, height: 1080 },
  { label: '480p · 16:9 (854×480)', width: 854, height: 480 },
  { label: 'Вертикальное · 9:16 (720×1280)', width: 720, height: 1280 },
  { label: 'Квадрат · 1:1 (720×720)', width: 720, height: 720 },
]

const CATEGORIES: Category[] = ['music', 'gaming', 'education', 'tech', 'entertainment', 'sports', 'news', 'other']

export function ExportDialog({ projectId, defaultTitle, onClose, onBeforeExport }: ExportDialogProps) {
  const navigate = useNavigate()
  const [resIdx, setResIdx] = useState(0)
  const [settings, setSettings] = useState<ExportSettings>({
    title: defaultTitle,
    description: '',
    category: 'other',
    visibility: 'public',
    tags: [],
    allowRemix: false,
    width: RESOLUTIONS[0].width,
    height: RESOLUTIONS[0].height,
    fps: 30,
    quality: 'medium',
    format: 'mp4',
  })
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState<number | null>(null)
  const [status, setStatus] = useState<RenderStatus | null>(null)
  const [resultVideoId, setResultVideoId] = useState<string | null>(null)
  const pollRef = useRef<number | null>(null)

  useEffect(() => {
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current)
    }
  }, [])

  function patch(p: Partial<ExportSettings>) {
    setSettings((s) => ({ ...s, ...p }))
  }

  async function start() {
    setBusy(true)
    setStatus(null)
    setProgress(0)
    try {
      await onBeforeExport()
      const { job, resultVideoId } = await exportProject(projectId, settings)
      setResultVideoId(resultVideoId)
      setStatus(job.status)
      poll(job.id)
    } catch (e) {
      setBusy(false)
      setProgress(null)
      const msg = e instanceof ApiError ? e.message : 'Не удалось запустить экспорт'
      toast(msg, 'error')
    }
  }

  function poll(jobId: string) {
    if (pollRef.current) window.clearInterval(pollRef.current)
    pollRef.current = window.setInterval(async () => {
      try {
        const job = await getExportJob(projectId, jobId)
        setProgress(job.progress)
        setStatus(job.status)
        if (job.status === 'completed') {
          stopPolling()
          toast('Экспорт завершён — создано новое видео')
        } else if (job.status === 'failed') {
          stopPolling()
          toast(job.error || 'Рендер завершился ошибкой', 'error')
        }
      } catch {
        /* временная ошибка сети — следующий тик повторит */
      }
    }, 1200)
  }

  function stopPolling() {
    if (pollRef.current) window.clearInterval(pollRef.current)
    pollRef.current = null
    setBusy(false)
  }

  const done = status === 'completed'
  const failed = status === 'failed'

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/60 p-4" onMouseDown={onClose}>
      <div
        className="w-full max-w-lg rounded-2xl bg-surface border border-border shadow-xl max-h-[90vh] overflow-y-auto"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-border">
          <h2 className="font-semibold text-text">Экспорт видео</h2>
          <button onClick={onClose} className="text-muted hover:text-text">
            <X className="w-5 h-5" />
          </button>
        </div>

        {progress === null ? (
          <div className="p-4 space-y-3 text-sm">
            <Field label="Название">
              <input
                value={settings.title}
                onChange={(e) => patch({ title: e.target.value })}
                className="w-full h-9 rounded-md bg-elevated border border-border px-2 text-text"
              />
            </Field>
            <Field label="Описание">
              <textarea
                value={settings.description}
                onChange={(e) => patch({ description: e.target.value })}
                rows={2}
                className="w-full rounded-md bg-elevated border border-border px-2 py-1.5 text-text resize-none"
              />
            </Field>

            <div className="grid grid-cols-2 gap-3">
              <Field label="Разрешение / формат кадра">
                <select
                  value={resIdx}
                  onChange={(e) => {
                    const i = Number(e.target.value)
                    setResIdx(i)
                    patch({ width: RESOLUTIONS[i].width, height: RESOLUTIONS[i].height })
                  }}
                  className="w-full h-9 rounded-md bg-elevated border border-border px-2 text-text"
                >
                  {RESOLUTIONS.map((r, i) => (
                    <option key={r.label} value={i}>
                      {r.label}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Частота кадров">
                <select
                  value={settings.fps}
                  onChange={(e) => patch({ fps: Number(e.target.value) })}
                  className="w-full h-9 rounded-md bg-elevated border border-border px-2 text-text"
                >
                  <option value={24}>24 fps</option>
                  <option value={30}>30 fps</option>
                  <option value={60}>60 fps</option>
                </select>
              </Field>
              <Field label="Качество">
                <select
                  value={settings.quality}
                  onChange={(e) => patch({ quality: e.target.value as ExportSettings['quality'] })}
                  className="w-full h-9 rounded-md bg-elevated border border-border px-2 text-text"
                >
                  <option value="low">Низкое (меньше размер)</option>
                  <option value="medium">Среднее</option>
                  <option value="high">Высокое</option>
                </select>
              </Field>
              <Field label="Формат">
                <select
                  value={settings.format}
                  onChange={(e) => patch({ format: e.target.value as ExportSettings['format'] })}
                  className="w-full h-9 rounded-md bg-elevated border border-border px-2 text-text"
                >
                  <option value="mp4">MP4 (H.264)</option>
                  <option value="webm">WebM (VP9)</option>
                </select>
              </Field>
              <Field label="Категория">
                <select
                  value={settings.category}
                  onChange={(e) => patch({ category: e.target.value as Category })}
                  className="w-full h-9 rounded-md bg-elevated border border-border px-2 text-text"
                >
                  {CATEGORIES.map((c) => (
                    <option key={c} value={c}>
                      {c}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Видимость">
                <select
                  value={settings.visibility}
                  onChange={(e) => patch({ visibility: e.target.value as Visibility })}
                  className="w-full h-9 rounded-md bg-elevated border border-border px-2 text-text"
                >
                  <option value="public">Публичное</option>
                  <option value="private">Приватное</option>
                </select>
              </Field>
            </div>

            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={settings.allowRemix}
                onChange={(e) => patch({ allowRemix: e.target.checked })}
              />
              Разрешить другим создавать свою версию (ремикс)
            </label>

            <div className="flex justify-end gap-2 pt-2">
              <Button variant="ghost" onClick={onClose}>
                Отмена
              </Button>
              <Button variant="primary" disabled={busy || !settings.title.trim()} onClick={start}>
                {busy ? <Loader2 className="w-4 h-4 animate-spin" /> : null}
                Экспортировать
              </Button>
            </div>
          </div>
        ) : (
          <div className="p-6 space-y-4 text-sm">
            {!done && !failed && (
              <>
                <div className="flex items-center gap-2 text-text">
                  <Loader2 className="w-5 h-5 animate-spin text-brand" />
                  Рендеринг на сервере… не закрывайте до завершения или вернитесь позже.
                </div>
                <div className="h-3 rounded-full bg-elevated overflow-hidden">
                  <div className="h-full bg-brand transition-all" style={{ width: `${progress ?? 0}%` }} />
                </div>
                <p className="text-muted tabular-nums">{progress ?? 0}%</p>
              </>
            )}
            {done && (
              <div className="space-y-4">
                <div className="flex items-center gap-2 text-green-500">
                  <CheckCircle2 className="w-6 h-6" /> Готово! Создано новое видео на вашем канале.
                </div>
                <div className="flex justify-end gap-2">
                  <Button variant="ghost" onClick={onClose}>
                    Закрыть
                  </Button>
                  {resultVideoId && (
                    <Button variant="primary" onClick={() => navigate(`/watch/${resultVideoId}`)}>
                      Открыть видео
                    </Button>
                  )}
                </div>
              </div>
            )}
            {failed && (
              <div className="space-y-4">
                <p className="text-danger">Рендер завершился ошибкой. Попробуйте снова — задача будет создана заново.</p>
                <div className="flex justify-end gap-2">
                  <Button variant="ghost" onClick={onClose}>
                    Закрыть
                  </Button>
                  <Button
                    variant="primary"
                    onClick={() => {
                      setProgress(null)
                      setStatus(null)
                    }}
                  >
                    Настроить заново
                  </Button>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs text-muted">{label}</span>
      {children}
    </label>
  )
}
