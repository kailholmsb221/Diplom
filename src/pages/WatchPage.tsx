import { useMemo, useRef } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ThumbsUp, ThumbsDown } from 'lucide-react'
import {
  getRecommended,
  getVideoById,
  getUserReaction,
  setReaction,
  incrementViews,
} from '@/api/videos'
import { getChannelById } from '@/api/channels'
import { fetchActiveAd } from '@/api/ads'
import { fetchSubtitles } from '@/api/transcripts'
import { useAuthStore } from '@/stores/authStore'
import { VideoPlayer, type PlayerHandle } from '@/features/video/player/VideoPlayer'
import { VideoCard } from '@/features/video/VideoCard'
import { SubscribeButton } from '@/features/channel/SubscribeButton'
import { CommentForm } from '@/features/comment/CommentForm'
import { CommentList } from '@/features/comment/CommentList'
import { TranscriptPanel } from '@/features/transcript/TranscriptPanel'
import { ChaptersList } from '@/features/transcript/ChaptersList'
import { RemixActions } from '@/features/editor/RemixActions'
import { Avatar } from '@/shared/ui/Avatar'
import { Loader, ErrorState } from '@/shared/ui/states'
import { formatDate, formatNumber, formatSubscribers, formatViews } from '@/shared/lib/format'
import { cn } from '@/shared/lib/cn'
import { useIsAuthenticated } from '@/stores/authStore'
import { requireAuth } from '@/features/auth/AuthPrompt'
import type { TranscriptLanguage } from '@/shared/types'

const LANG_LABELS: Record<TranscriptLanguage, string> = {
  ru: 'Русский',
  kk: 'Қазақша',
  en: 'English',
}

export function WatchPage() {
  const { videoId = '' } = useParams()
  const qc = useQueryClient()
  const isAuthed = useIsAuthenticated()
  const playerRef = useRef<PlayerHandle | null>(null)

  const isPremium = useAuthStore((s) => s.user?.premium ?? false)
  const currentUserId = useAuthStore((s) => s.user?.id)
  const { data: ad } = useQuery({
    queryKey: ['ad', 'active'],
    queryFn: fetchActiveAd,
    enabled: !isPremium,
    staleTime: 60_000,
  })

  const { data: video, isLoading, isError, refetch } = useQuery({
    queryKey: ['video', videoId],
    queryFn: () => getVideoById(videoId),
    // Пока транскрипция идёт — подтягиваем главы.
    refetchInterval: (q) => {
      const s = q.state.data?.transcriptStatus
      return s === 'PROCESSING' || s === 'TRANSLATING' ? 5_000 : false
    },
  })
  const { data: channel } = useQuery({
    queryKey: ['channel', video?.channelId],
    queryFn: () => (video ? getChannelById(video.channelId) : null),
    enabled: !!video,
  })
  const { data: reaction } = useQuery({
    queryKey: ['reaction', videoId],
    queryFn: () => getUserReaction(videoId),
    enabled: isAuthed,
  })
  const { data: recommended } = useQuery({
    queryKey: ['recommended', videoId],
    queryFn: () => getRecommended(videoId, 10),
  })
  const { data: subtitles } = useQuery({
    queryKey: ['subtitles', videoId],
    queryFn: () => fetchSubtitles(videoId),
    refetchInterval: (q) => {
      const s = q.state.data?.videoStatus
      return s === 'PROCESSING' || s === 'TRANSLATING' ? 5_000 : false
    },
  })

  const subtitleTracks = useMemo(() => {
    if (!subtitles) return []
    return subtitles.availableLanguages.map((lang) => ({
      language: lang,
      label: LANG_LABELS[lang],
      src: subtitles.subtitleUrls[lang] ?? '',
    }))
  }, [subtitles])

  const reactMutation = useMutation({
    mutationFn: async (next: 'like' | 'dislike' | null) => setReaction(videoId, next),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['reaction', videoId] })
      qc.invalidateQueries({ queryKey: ['video', videoId] })
    },
  })

  function toggleReaction(next: 'like' | 'dislike') {
    const verb = next === 'like' ? 'поставить лайк' : 'поставить дизлайк'
    if (!requireAuth(`Чтобы ${verb}, зарегистрируйтесь или войдите.`)) return
    reactMutation.mutate(reaction === next ? null : next)
  }

  function seekTo(seconds: number) {
    playerRef.current?.seekTo(seconds)
  }

  if (isLoading) return <Loader />
  if (isError || !video)
    return (
      <ErrorState
        title="Видео не найдено"
        message="Возможно, оно было удалено или перемещено."
        onRetry={() => refetch()}
      />
    )

  return (
    <div className="px-4 lg:px-6 py-4 lg:flex lg:gap-6 max-w-[1800px] mx-auto">
      <div className="flex-1 min-w-0">
        <div className="rounded-xl overflow-hidden bg-black">
          <VideoPlayer
            ref={playerRef}
            video={video}
            preRollAd={isPremium ? null : ad}
            subtitleTracks={subtitleTracks}
            onFirstPlay={() => {
              if (isAuthed) void incrementViews(videoId)
            }}
          />
        </div>

        <h1 className="text-xl font-semibold mt-4">{video.title}</h1>

        <div className="flex flex-wrap items-center gap-4 mt-3">
          <Link to={`/channel/${video.channelId}`} className="flex items-center gap-3 min-w-0">
            <Avatar src={channel?.avatarUrl} alt={channel?.name} size={42} />
            <div className="min-w-0">
              <div className="font-medium truncate">{channel?.name ?? '—'}</div>
              <div className="text-xs text-muted">
                {channel ? formatSubscribers(channel.subscribersCount) : '—'}
              </div>
            </div>
          </Link>

          {channel && <SubscribeButton channelId={channel.id} />}

          <div className="flex items-center gap-1 ml-auto bg-elevated rounded-full overflow-hidden">
            <button
              onClick={() => toggleReaction('like')}
              className={cn(
                'flex items-center gap-2 px-4 h-10 hover:bg-border',
                reaction === 'like' && 'text-brand',
              )}
            >
              <ThumbsUp className="w-5 h-5" />
              <span className="text-sm">{formatNumber(video.likes)}</span>
            </button>
            <div className="w-px h-5 bg-border" />
            <button
              onClick={() => toggleReaction('dislike')}
              className={cn(
                'flex items-center gap-2 px-4 h-10 hover:bg-border',
                reaction === 'dislike' && 'text-brand',
              )}
            >
              <ThumbsDown className="w-5 h-5" />
              <span className="text-sm">{formatNumber(video.dislikes)}</span>
            </button>
          </div>
        </div>

        <div className="mt-4 p-3 bg-surface rounded-xl">
          <div className="flex flex-wrap gap-3 text-sm font-medium">
            <span>{formatViews(video.views)}</span>
            <span>{formatDate(video.uploadedAt)}</span>
            {video.tags.length > 0 && (
              <span className="text-muted">
                {video.tags.map((t) => `#${t.replace(/^#+/, '')}`).join(' ')}
              </span>
            )}
          </div>
          <p className="text-sm mt-2 whitespace-pre-wrap">{video.description}</p>
          <ChaptersList chapters={video.chapters ?? []} onSeek={seekTo} />
        </div>

        <div className="mt-3">
          <RemixActions video={video} isOwner={!!currentUserId && channel?.ownerId === currentUserId} />
        </div>

        <section className="mt-6">
          <TranscriptPanel videoId={videoId} onSeek={seekTo} />
        </section>

        <section className="mt-8">
          <h2 className="text-base font-semibold mb-4">Комментарии</h2>
          <div className="mb-6">
            <CommentForm videoId={videoId} />
          </div>
          <CommentList videoId={videoId} />
        </section>
      </div>

      <aside className="lg:w-96 mt-8 lg:mt-0 flex-shrink-0">
        <h3 className="text-sm font-semibold mb-3">Рекомендации</h3>
        <div className="space-y-3">
          {recommended?.map((v) => <VideoCard key={v.id} video={v} layout="compact" />)}
        </div>
      </aside>
    </div>
  )
}
