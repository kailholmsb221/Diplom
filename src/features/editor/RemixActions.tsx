import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Scissors, Sparkles } from 'lucide-react'
import { Button } from '@/shared/ui/Button'
import { toast } from '@/shared/ui/toast'
import { createProject, setRemixPermission } from '@/api/editor'
import { requireAuth } from '@/features/auth/AuthPrompt'
import type { Video } from '@/shared/types'

interface RemixActionsProps {
  video: Video
  isOwner: boolean
}

/** Кнопки «Редактировать» / «Создать свою версию» + переключатель разрешения ремикса. */
export function RemixActions({ video, isOwner }: RemixActionsProps) {
  const navigate = useNavigate()
  const qc = useQueryClient()

  const edit = useMutation({
    mutationFn: () => createProject({ sourceVideoId: video.id, isRemix: false }),
    onSuccess: (p) => navigate(`/me/editor/${p.id}`),
    onError: (e) => toast(e instanceof Error ? e.message : 'Не удалось открыть редактор', 'error'),
  })

  const remix = useMutation({
    mutationFn: () => createProject({ sourceVideoId: video.id, isRemix: true }),
    onSuccess: (p) => navigate(`/me/editor/${p.id}`),
    onError: (e) => toast(e instanceof Error ? e.message : 'Не удалось создать версию', 'error'),
  })

  const togglePermission = useMutation({
    mutationFn: (allow: boolean) => setRemixPermission(video.id, allow),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['video', video.id] })
      toast(video.allowRemix ? 'Ремикс запрещён' : 'Ремикс разрешён')
    },
    onError: (e) => toast(e instanceof Error ? e.message : 'Не удалось изменить настройку', 'error'),
  })

  function startRemix() {
    if (!requireAuth('Чтобы создать свою версию, войдите или зарегистрируйтесь.')) return
    remix.mutate()
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {isOwner ? (
        <>
          <Button size="sm" variant="subtle" disabled={edit.isPending} onClick={() => edit.mutate()}>
            <Scissors className="w-4 h-4" /> Редактировать
          </Button>
          <Button
            size="sm"
            variant={video.allowRemix ? 'primary' : 'ghost'}
            disabled={togglePermission.isPending}
            onClick={() => togglePermission.mutate(!video.allowRemix)}
            title="Разрешить другим пользователям делать свои версии вашего видео"
          >
            <Sparkles className="w-4 h-4" />
            {video.allowRemix ? 'Ремикс разрешён' : 'Разрешить ремикс'}
          </Button>
        </>
      ) : (
        video.allowRemix && (
          <Button size="sm" variant="subtle" disabled={remix.isPending} onClick={startRemix}>
            <Sparkles className="w-4 h-4" /> Создать свою версию
          </Button>
        )
      )}

      {video.sourceChannelId && video.sourceChannelName && (
        <span className="text-xs text-muted">
          Оригинал:{' '}
          <Link to={`/channel/${video.sourceChannelId}`} className="text-brand hover:underline">
            {video.sourceChannelName}
          </Link>
        </span>
      )}
    </div>
  )
}
