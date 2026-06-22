import { useRef, useState, type Dispatch } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Film, Info, Link2, Music2, Upload } from 'lucide-react'
import { Button } from '@/shared/ui/Button'
import { api } from '@/api/client'
import { getChannelByOwner } from '@/api/channels'
import { uploadAudioFile, uploadVideoFile } from '@/api/videos'
import { toast } from '@/shared/ui/toast'
import { useAuthStore } from '@/stores/authStore'
import { formatDuration } from '@/shared/lib/format'
import { type EngineAction } from './useEditorEngine'
import { probeDuration } from './mediaProbe'

interface MediaItem {
  id: string
  title: string
  videoUrl: string
  durationSec: number
  thumbnailUrl: string
}

interface MediaLibraryProps {
  dispatch: Dispatch<EngineAction>
}

export function MediaLibrary({ dispatch }: MediaLibraryProps) {
  const userId = useAuthStore((s) => s.user?.id)
  const fileRef = useRef<HTMLInputElement>(null)
  const audioRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)
  const [uploadingAudio, setUploadingAudio] = useState(false)
  const [linkValue, setLinkValue] = useState('')
  const [linkError, setLinkError] = useState('')

  const channelQ = useQuery({
    queryKey: ['my-channel', userId],
    queryFn: () => getChannelByOwner(userId!),
    enabled: !!userId,
  })

  const channelId = channelQ.data?.id
  const videosQ = useQuery({
    queryKey: ['editor-media', channelId],
    queryFn: () => api<MediaItem[]>('/api/videos', { query: { channelId, limit: 100 } }),
    enabled: !!channelId,
  })

  async function addClip(url: string, name: string, knownDuration?: number) {
    const duration = knownDuration && knownDuration > 0 ? knownDuration : await probeDuration(url)
    const safe = duration > 0 ? duration : 30
    dispatch({
      type: 'addVideoClip',
      clip: { sourceUrl: url, name, sourceDuration: safe, trimStart: 0, trimEnd: safe },
    })
    toast(`Добавлено: ${name}`)
  }

  async function onUpload(file: File) {
    setUploading(true)
    try {
      const url = await uploadVideoFile(file)
      await addClip(url, file.name)
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось загрузить файл', 'error')
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  /** Добавляет аудиоклип (музыка/озвучка) на аудиодорожку с начала таймлайна. */
  async function addAudioClip(url: string, name: string, knownDuration?: number) {
    const duration = knownDuration && knownDuration > 0 ? knownDuration : await probeDuration(url, 'audio')
    const safe = duration > 0 ? duration : 30
    dispatch({
      type: 'addAudioClip',
      clip: { sourceUrl: url, name, sourceDuration: safe, trimStart: 0, trimEnd: safe, start: 0 },
    })
    toast(`Звук добавлен: ${name}`)
  }

  async function onUploadAudio(file: File) {
    setUploadingAudio(true)
    try {
      const url = await uploadAudioFile(file)
      await addAudioClip(url, file.name)
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось загрузить звук', 'error')
    } finally {
      setUploadingAudio(false)
      if (audioRef.current) audioRef.current.value = ''
    }
  }

  /** Берёт звуковую дорожку своего видео как аудиоклип (для подложки/озвучки). */
  async function addAudioFromVideo(v: MediaItem) {
    const safe = v.durationSec > 0 ? v.durationSec : (await probeDuration(v.videoUrl)) || 30
    dispatch({
      type: 'addAudioClip',
      clip: {
        sourceUrl: v.videoUrl,
        name: (v.title || 'Видео') + ' (звук)',
        sourceDuration: safe,
        trimStart: 0,
        trimEnd: safe,
        start: 0,
      },
    })
    toast(`Звук добавлен: ${v.title || 'видео'}`)
  }

  function onImportLink() {
    const v = linkValue.trim()
    setLinkError('')
    if (!v) return
    // Легальный шлюз: внешние видео (включая YouTube) импортировать нельзя — без
    // обхода защиты и нарушения авторских прав. Объясняем и предлагаем альтернативы.
    setLinkError(
      'Импорт по внешним ссылкам (в т.ч. YouTube) недоступен: это нарушало бы авторские права и условия сервиса. ' +
        'Легальные варианты: загрузите свой файл, выберите своё видео ниже или используйте чужое видео с включённым автором ремиксом («Создать свою версию»).',
    )
  }

  return (
    <div className="flex flex-col h-full text-sm">
      <div className="p-3 border-b border-border space-y-2">
        <h3 className="font-medium text-text flex items-center gap-1.5">
          <Film className="w-4 h-4" /> Медиатека
        </h3>
        <input
          ref={fileRef}
          type="file"
          accept="video/mp4,video/webm"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0]
            if (f) onUpload(f)
          }}
        />
        <Button
          variant="primary"
          size="sm"
          className="w-full"
          disabled={uploading}
          onClick={() => fileRef.current?.click()}
        >
          <Upload className="w-4 h-4" /> {uploading ? 'Загрузка…' : 'Импорт видео (mp4/webm)'}
        </Button>

        <input
          ref={audioRef}
          type="file"
          accept="audio/*"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0]
            if (f) onUploadAudio(f)
          }}
        />
        <Button
          variant="subtle"
          size="sm"
          className="w-full"
          disabled={uploadingAudio}
          onClick={() => audioRef.current?.click()}
        >
          <Music2 className="w-4 h-4" /> {uploadingAudio ? 'Загрузка…' : 'Импорт звука (mp3/wav)'}
        </Button>
      </div>

      {/* Импорт по ссылке — легальный шлюз */}
      <div className="p-3 border-b border-border space-y-2">
        <label className="text-xs text-muted flex items-center gap-1.5">
          <Link2 className="w-3.5 h-3.5" /> Импорт по ссылке
        </label>
        <div className="flex gap-1.5">
          <input
            value={linkValue}
            onChange={(e) => setLinkValue(e.target.value)}
            placeholder="https://…"
            className="flex-1 h-8 rounded-md bg-surface border border-border px-2 text-text min-w-0"
          />
          <Button size="sm" variant="subtle" onClick={onImportLink}>
            ОК
          </Button>
        </div>
        {linkError && (
          <p className="text-xs text-amber-600 dark:text-amber-400 flex gap-1.5">
            <Info className="w-4 h-4 shrink-0 mt-0.5" />
            <span>{linkError}</span>
          </p>
        )}
      </div>

      {/* Свои видео */}
      <div className="flex-1 overflow-y-auto p-3 space-y-2">
        <h4 className="text-xs text-muted uppercase tracking-wide">Мои видео</h4>
        {videosQ.isLoading && <p className="text-xs text-muted">Загрузка…</p>}
        {videosQ.data?.length === 0 && <p className="text-xs text-muted">У вас пока нет видео.</p>}
        {videosQ.data
          ?.filter((v) => v.videoUrl)
          .map((v) => (
            <div key={v.id} className="flex gap-1 items-center rounded-lg hover:bg-elevated">
              <button
                onClick={() => addClip(v.videoUrl, v.title || 'Видео', v.durationSec)}
                className="flex-1 min-w-0 flex gap-2 items-center p-1.5 text-left"
                title="Добавить как видео"
              >
                <div className="w-16 h-10 rounded bg-black/40 overflow-hidden shrink-0">
                  {v.thumbnailUrl && <img src={v.thumbnailUrl} alt="" className="w-full h-full object-cover" />}
                </div>
                <div className="min-w-0">
                  <p className="truncate text-text">{v.title || 'Без названия'}</p>
                  <p className="text-xs text-muted">{formatDuration(v.durationSec)}</p>
                </div>
              </button>
              <button
                onClick={() => addAudioFromVideo(v)}
                className="shrink-0 p-2 mr-1 rounded-md text-muted hover:text-text hover:bg-surface"
                title="Добавить только звук этого видео на аудиодорожку"
              >
                <Music2 className="w-4 h-4" />
              </button>
            </div>
          ))}
      </div>
    </div>
  )
}
