import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Scissors, Trash2 } from 'lucide-react'
import { deleteVideo, fetchVideos } from '@/api/videos'
import { getChannelByOwner } from '@/api/channels'
import { getCurrentUser } from '@/api/users'
import { createProject } from '@/api/editor'
import { Button } from '@/shared/ui/Button'
import { Loader, EmptyState } from '@/shared/ui/states'
import { toast } from '@/shared/ui/toast'
import { formatDuration, formatNumber, timeAgo } from '@/shared/lib/format'

export function MyVideosPage() {
  const qc = useQueryClient()
  const { data: user } = useQuery({ queryKey: ['currentUser'], queryFn: getCurrentUser })
  const { data: channel } = useQuery({
    queryKey: ['channel', 'owner', user?.id],
    queryFn: () => (user ? getChannelByOwner(user.id) : null),
    enabled: !!user,
  })
  const { data: videos, isLoading } = useQuery({
    queryKey: ['videos', { channelId: channel?.id }],
    queryFn: () => fetchVideos({ channelId: channel!.id }),
    enabled: !!channel,
  })

  const navigate = useNavigate()
  const remove = useMutation({
    mutationFn: (id: string) => deleteVideo(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['videos'] }),
  })

  const openEditor = useMutation({
    mutationFn: (videoId: string) => createProject({ sourceVideoId: videoId, isRemix: false }),
    onSuccess: (project) => navigate(`/me/editor/${project.id}`),
    onError: (e) => toast(e instanceof Error ? e.message : 'Не удалось открыть редактор', 'error'),
  })

  const isAdmin = user?.role === 'admin'

  return (
    <div className="px-4 lg:px-8 py-6 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Мой канал</h1>
        {!isAdmin && (
        <Link to="/me/upload">
          <Button>Загрузить новое</Button>
        </Link>
        )}
      </div>

      {isLoading && <Loader />}
      {!isLoading && (!videos || videos.length === 0) && (
        <EmptyState
          title="Вы ещё не загружали видео"
          message="Нажмите «Загрузить новое», чтобы начать."
        />
      )}
      {!isLoading && videos && videos.length > 0 && (
        <div className="bg-surface border border-border rounded-2xl overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-elevated text-left">
              <tr>
                <th className="px-4 py-3">Видео</th>
                <th className="px-4 py-3">Доступ</th>
                <th className="px-4 py-3">Дата</th>
                <th className="px-4 py-3">Просмотры</th>
                <th className="px-4 py-3">Лайки</th>
                <th className="px-4 py-3"></th>
              </tr>
            </thead>
            <tbody>
              {videos.map((v) => (
                <tr key={v.id} className="border-t border-border hover:bg-elevated/50">
                  <td className="px-4 py-3">
                    <Link to={`/watch/${v.id}`} className="flex items-center gap-3 hover:text-brand">
                      <div className="relative w-24 flex-shrink-0">
                        <img
                          src={v.thumbnailUrl}
                          alt={v.title}
                          className="w-full aspect-video object-cover rounded"
                        />
                        <span className="absolute bottom-1 right-1 bg-black/80 text-white text-[10px] px-1 rounded">
                          {formatDuration(v.durationSec)}
                        </span>
                      </div>
                      <span className="line-clamp-2 max-w-xs">{v.title}</span>
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={`px-2 py-0.5 rounded text-xs font-medium ${
                        v.visibility === 'public'
                          ? 'bg-green-500/20 text-green-700 dark:text-green-300'
                          : 'bg-yellow-500/20 text-yellow-700 dark:text-yellow-300'
                      }`}
                    >
                      {v.visibility === 'public' ? 'Публичное' : 'Приватное'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-muted">{timeAgo(v.uploadedAt)}</td>
                  <td className="px-4 py-3">{formatNumber(v.views)}</td>
                  <td className="px-4 py-3">{formatNumber(v.likes)}</td>
                  <td className="px-4 py-3 text-right whitespace-nowrap">
                    <button
                      onClick={() => openEditor.mutate(v.id)}
                      disabled={openEditor.isPending || !v.sources.length}
                      className="p-2 rounded hover:bg-brand/10 hover:text-brand disabled:opacity-40"
                      aria-label="Редактировать в видеоредакторе"
                      title="Редактировать"
                    >
                      <Scissors className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => {
                        if (confirm(`Удалить «${v.title}»?`)) remove.mutate(v.id)
                      }}
                      className="p-2 rounded hover:bg-danger/10 hover:text-danger"
                      aria-label="Удалить"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
