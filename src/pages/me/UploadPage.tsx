import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { UploadCloud } from 'lucide-react'
import { uploadImageFile, uploadVideo, uploadVideoFile } from '@/api/videos'
import { createProject } from '@/api/editor'
import { getChannelByOwner } from '@/api/channels'
import { getCurrentUser } from '@/api/users'
import { Input, Textarea } from '@/shared/ui/Input'
import { Button } from '@/shared/ui/Button'
import type { Category, Visibility } from '@/shared/types'

const categories: { value: Category; label: string }[] = [
  { value: 'tech', label: 'Технологии' },
  { value: 'music', label: 'Музыка' },
  { value: 'gaming', label: 'Игры' },
  { value: 'education', label: 'Образование' },
  { value: 'entertainment', label: 'Развлечения' },
  { value: 'sports', label: 'Спорт' },
  { value: 'news', label: 'Новости' },
  { value: 'other', label: 'Другое' },
]

const schema = z.object({
  title: z.string().trim().min(1, 'Введите название').max(120, 'Не более 120 символов'),
  description: z.string().trim().max(5000, 'Не более 5000 символов').optional(),
  category: z.enum(['tech', 'music', 'gaming', 'education', 'entertainment', 'sports', 'news', 'other']),
  visibility: z.enum(['public', 'private']),
  tags: z.string().optional(),
})

type FormValues = z.infer<typeof schema>

function getVideoDuration(file: File): Promise<number> {
  return new Promise((resolve, reject) => {
    const video = document.createElement('video')
    const url = URL.createObjectURL(file)

    video.preload = 'metadata'
    video.onloadedmetadata = () => {
      URL.revokeObjectURL(url)
      resolve(Math.max(0, Math.round(video.duration)))
    }
    video.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('Could not read video duration'))
    }
    video.src = url
  })
}

function parseTags(raw = ''): string[] {
  return raw
    .split(/[,\s]+/)
    .map((tag) => tag.trim().replace(/^#+/, ''))
    .filter(Boolean)
}

export function UploadPage() {
  const navigate = useNavigate()
  const [videoFile, setVideoFile] = useState<File | null>(null)
  const [thumbFile, setThumbFile] = useState<File | null>(null)
  const [thumbPreview, setThumbPreview] = useState<string>('')
  const [submitError, setSubmitError] = useState('')
  const [openInEditor, setOpenInEditor] = useState(false)

  const { data: user } = useQuery({ queryKey: ['currentUser'], queryFn: getCurrentUser })
  const { data: channel } = useQuery({
    queryKey: ['channel', 'owner', user?.id],
    queryFn: () => (user ? getChannelByOwner(user.id) : null),
    enabled: !!user,
  })

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { category: 'other', visibility: 'public' },
  })

  const upload = useMutation({
    mutationFn: async (data: FormValues) => {
      if (!videoFile) throw new Error('Выберите видеофайл')
      if (!thumbFile) throw new Error('Выберите превью')
      if (!channel) throw new Error('Канал не найден')
      const durationSec = await getVideoDuration(videoFile)
      const [videoUrl, thumbUrl] = await Promise.all([
        uploadVideoFile(videoFile),
        uploadImageFile(thumbFile),
      ])
      return uploadVideo({
        channelId: channel.id,
        title: data.title,
        description: data.description ?? '',
        thumbnailUrl: thumbUrl,
        videoFileUrl: videoUrl,
        durationSec,
        category: data.category as Category,
        visibility: data.visibility as Visibility,
        tags: parseTags(data.tags),
      })
    },
    onSuccess: async (video) => {
      if (openInEditor) {
        try {
          const project = await createProject({ sourceVideoId: video.id, isRemix: false })
          navigate(`/me/editor/${project.id}`)
          return
        } catch {
          /* при ошибке создания проекта просто уходим в список видео */
        }
      }
      navigate('/me/videos')
    },
    onError: (e: Error) => setSubmitError(e.message),
  })

  function onThumbChange(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (!f) return
    setThumbFile(f)
    const reader = new FileReader()
    reader.onload = () => setThumbPreview(reader.result as string)
    reader.readAsDataURL(f)
  }

  if (user?.role === 'admin') {
    return (
      <div className="px-4 lg:px-8 py-6 max-w-3xl mx-auto">
        <div className="bg-surface border border-border rounded-2xl p-6">
          <h1 className="text-2xl font-bold mb-2">Загрузка видео недоступна</h1>
          <p className="text-sm text-muted mb-4">
            Администратор может добавлять только рекламные ролики через админ-панель.
          </p>
          <Button onClick={() => navigate('/admin/ads')}>Перейти к рекламе</Button>
        </div>
      </div>
    )
  }

  return (
    <div className="px-4 lg:px-8 py-6 max-w-3xl mx-auto">
      <h1 className="text-2xl font-bold mb-6">Загрузка видео</h1>

      <form onSubmit={handleSubmit((data) => upload.mutate(data))} className="space-y-5">
        <div className="bg-surface border border-border rounded-2xl p-6">
          <label className="block text-sm font-medium mb-2">Видеофайл *</label>
          <label className="flex flex-col items-center justify-center gap-2 border-2 border-dashed border-border rounded-xl p-8 cursor-pointer hover:bg-elevated">
            <UploadCloud className="w-10 h-10 text-muted" />
            <span className="text-sm text-muted">
              {videoFile ? videoFile.name : 'Нажмите, чтобы выбрать видеофайл'}
            </span>
            <input
              type="file"
              accept="video/*"
              className="hidden"
              onChange={(e) => setVideoFile(e.target.files?.[0] ?? null)}
            />
          </label>
        </div>

        <div className="bg-surface border border-border rounded-2xl p-6">
          <label className="block text-sm font-medium mb-2">Превью *</label>
          <div className="flex items-start gap-4">
            <label className="flex flex-col items-center justify-center gap-2 border-2 border-dashed border-border rounded-xl p-6 cursor-pointer hover:bg-elevated flex-1">
              <UploadCloud className="w-8 h-8 text-muted" />
              <span className="text-sm text-muted">{thumbFile ? thumbFile.name : 'Выбрать превью'}</span>
              <input type="file" accept="image/*" className="hidden" onChange={onThumbChange} />
            </label>
            {thumbPreview && (
              <img src={thumbPreview} alt="превью" className="w-48 aspect-video object-cover rounded-lg" />
            )}
          </div>
        </div>

        <div className="bg-surface border border-border rounded-2xl p-6 space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Название *</label>
            <Input {...register('title')} placeholder="Заголовок видео" />
            {errors.title && <p className="text-danger text-xs mt-1">{errors.title.message}</p>}
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Описание</label>
            <Textarea {...register('description')} rows={4} placeholder="Расскажите о видео..." />
            {errors.description && <p className="text-danger text-xs mt-1">{errors.description.message}</p>}
          </div>
          <div className="grid sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Категория *</label>
              <select
                {...register('category')}
                className="h-10 w-full rounded-lg border border-border bg-surface px-3 text-sm"
              >
                {categories.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Доступность *</label>
              <select
                {...register('visibility')}
                className="h-10 w-full rounded-lg border border-border bg-surface px-3 text-sm"
              >
                <option value="public">Публичное</option>
                <option value="private">Приватное</option>
              </select>
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Теги</label>
            <Input {...register('tags')} placeholder="#react #tutorial #2026" />
          </div>
        </div>

        {submitError && <p className="text-danger text-sm">{submitError}</p>}

        <div className="flex flex-wrap justify-end gap-2">
          <Button type="button" variant="ghost" onClick={() => navigate(-1)}>
            Отмена
          </Button>
          <Button
            type="submit"
            variant="subtle"
            disabled={upload.isPending}
            onClick={() => setOpenInEditor(true)}
          >
            {upload.isPending ? 'Загрузка...' : 'Опубликовать и открыть в редакторе'}
          </Button>
          <Button type="submit" disabled={upload.isPending} onClick={() => setOpenInEditor(false)}>
            {upload.isPending ? 'Загрузка...' : 'Опубликовать'}
          </Button>
        </div>
      </form>
    </div>
  )
}
