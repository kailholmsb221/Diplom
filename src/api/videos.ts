import { api } from './client'
import type { Video } from '@/shared/types'

type Reaction = 'like' | 'dislike' | null

interface VideoApi extends Omit<Video, 'sources'> {
  videoUrl: string
}

function adapt(v: VideoApi): Video {
  return {
    ...v,
    // Backend пока хранит один файл на видео — в плеере отображается как «Авто».
    sources: v.videoUrl ? [{ quality: '720p', url: v.videoUrl }] : [],
  }
}

export async function fetchVideos(params: { q?: string; channelId?: string; limit?: number } = {}): Promise<Video[]> {
  const list = await api<VideoApi[]>('/api/videos', {
    query: { q: params.q, channelId: params.channelId, limit: params.limit, onlyPublic: true },
  })
  return list.map(adapt)
}

export async function getVideoById(id: string): Promise<Video | null> {
  try {
    const v = await api<VideoApi>(`/api/videos/${id}`)
    return adapt(v)
  } catch {
    return null
  }
}

export async function getRecommended(excludeId: string, limit = 10): Promise<Video[]> {
  const list = await api<VideoApi[]>(`/api/videos/${excludeId}/recommendations`, {
    query: { limit },
  })
  return list.map(adapt)
}

export async function getRecommendedHome(limit = 24): Promise<Video[]> {
  const list = await api<VideoApi[]>('/api/videos/recommended', { query: { limit } })
  return list.map(adapt)
}

export async function getUserReaction(videoId: string): Promise<Reaction> {
  try {
    const { reaction } = await api<{ reaction: string }>(`/api/videos/${videoId}/reaction`)
    if (reaction === 'like' || reaction === 'dislike') return reaction
    return null
  } catch {
    return null
  }
}

export async function setReaction(videoId: string, reaction: Reaction): Promise<void> {
  await api(`/api/videos/${videoId}/reaction`, {
    method: 'POST',
    body: { reaction: reaction ?? '' },
  })
}

interface UploadVideoInput {
  channelId: string
  title: string
  description: string
  thumbnailUrl: string
  videoFileUrl: string
  durationSec: number
  category: Video['category']
  visibility: Video['visibility']
  tags?: string[]
}

export async function uploadVideo(input: UploadVideoInput): Promise<Video> {
  const created = await api<VideoApi>('/api/videos', {
    method: 'POST',
    body: {
      channelId: input.channelId,
      title: input.title,
      description: input.description,
      thumbnailUrl: input.thumbnailUrl,
      videoUrl: input.videoFileUrl,
      durationSec: input.durationSec,
      category: input.category,
      visibility: input.visibility,
      tags: input.tags ?? [],
    },
  })
  return adapt(created)
}

export async function updateVideo(): Promise<void> {
  // Заглушка для совместимости; backend пока не имеет PATCH /videos/:id.
}

export async function deleteVideo(id: string): Promise<void> {
  await api(`/api/videos/${id}`, { method: 'DELETE' })
}

export async function incrementViews(id: string): Promise<void> {
  await api(`/api/videos/${id}/views`, { method: 'POST' })
}

export async function adminAdjustVideoStats(
  videoId: string,
  patch: { viewsDelta?: number; likesDelta?: number; dislikesDelta?: number },
): Promise<void> {
  await api(`/api/admin/videos/${videoId}/stats`, { method: 'POST', body: patch })
}

export async function uploadVideoFile(file: File): Promise<string> {
  const fd = new FormData()
  fd.append('file', file)
  const { url } = await api<{ url: string }>('/api/upload/video', { method: 'POST', body: fd })
  return url
}

export async function uploadImageFile(file: File): Promise<string> {
  const fd = new FormData()
  fd.append('file', file)
  const { url } = await api<{ url: string }>('/api/upload/image', { method: 'POST', body: fd })
  return url
}

export async function uploadAudioFile(file: File): Promise<string> {
  const fd = new FormData()
  fd.append('file', file)
  const { url } = await api<{ url: string }>('/api/upload/audio', { method: 'POST', body: fd })
  return url
}
