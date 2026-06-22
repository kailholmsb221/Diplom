export type Quality = '1080p' | '720p' | '480p' | '360p'

export type Category =
  | 'music'
  | 'gaming'
  | 'education'
  | 'tech'
  | 'entertainment'
  | 'sports'
  | 'news'
  | 'other'

export type Visibility = 'public' | 'private'

export interface User {
  id: string
  username: string
  displayName: string
  email: string
  avatarUrl: string
  bio?: string
  role: 'user' | 'admin'
  createdAt: string
  blocked?: boolean
  premium?: boolean
  premiumUntil?: string | null
}

export interface VideoSource {
  quality: Quality
  url: string
}

export interface Channel {
  id: string
  ownerId: string
  name: string
  handle: string
  description: string
  avatarUrl: string
  bannerUrl: string
  subscribersCount: number
  createdAt: string
  balance?: number
  totalEarned?: number
}

export type TranscriptLanguage = 'ru' | 'kk' | 'en'

export type TranscriptStatus =
  | 'NOT_STARTED'
  | 'PROCESSING'
  | 'TRANSLATING'
  | 'COMPLETED'
  | 'FAILED'

export interface TranscriptSegment {
  start: number
  end: number
  text: string
}

export interface Chapter {
  start: number
  title: string
}

export interface TranscriptResponse {
  videoId: string
  selectedLanguage: TranscriptLanguage
  originalLanguage: TranscriptLanguage | ''
  status: TranscriptStatus
  fullText: string
  segments: TranscriptSegment[]
  vttUrl?: string
  error?: string
  videoStatus: TranscriptStatus
}

export interface SubtitlesResponse {
  videoId: string
  originalLanguage: TranscriptLanguage | ''
  videoStatus: TranscriptStatus
  availableLanguages: TranscriptLanguage[]
  subtitleUrls: Partial<Record<TranscriptLanguage, string>>
  status: Partial<Record<TranscriptLanguage, TranscriptStatus>>
}

export interface Ad {
  id: string
  title: string
  description: string
  videoUrl: string
  active: boolean
  createdAt: string
  updatedAt: string
}

export type TransactionType =
  | 'PREMIUM_PURCHASE'
  | 'CHANNEL_PAYOUT'
  | 'ADMIN_ADJUSTMENT'

export type TransactionStatus = 'SUCCESS' | 'FAILED' | 'PENDING'

export interface Transaction {
  id: string
  userId?: string | null
  channelId?: string | null
  type: TransactionType
  amount: number
  status: TransactionStatus
  description: string
  cardLast4?: string | null
  createdAt: string
}

export interface Video {
  id: string
  channelId: string
  title: string
  description: string
  thumbnailUrl: string
  sources: VideoSource[]
  durationSec: number
  views: number
  likes: number
  dislikes: number
  uploadedAt: string
  tags: string[]
  category: Category
  visibility: Visibility
  transcriptStatus?: TranscriptStatus
  originalLanguage?: TranscriptLanguage | null
  chapters?: Chapter[]
  allowRemix?: boolean
  sourceVideoId?: string | null
  sourceChannelId?: string | null
  sourceChannelName?: string | null
}

// ----------------------------- Видеоредактор -----------------------------

/** Параметры цветокоррекции клипа. Нейтральные значения = картинка без изменений. */
export interface ClipColor {
  brightness: number // -1..1   (0 = норма)
  contrast: number //   0..2    (1 = норма)
  saturation: number // 0..3    (1 = норма)
  exposure: number //   0.1..3  (1 = норма)
  temperature: number // -100..100 (0 = норма)
  hue: number //        -180..180 (0 = норма)
  highlights: number // -1..1   (0 = норма)
  shadows: number //    -1..1   (0 = норма)
  sharpness: number //  0..2    (0 = норма)
  opacity: number //    0..1    (1 = норма)
}

export interface ClipAudio {
  muted: boolean
  volume: number // 0..2 (1 = норма)
  fadeIn: number // сек
  fadeOut: number // сек
}

export type TrackKind = 'video' | 'audio'

export interface TimelineClip {
  id: string
  sourceUrl: string
  /** Имя источника для подписи в UI. */
  name?: string
  /** Полная длительность исходного файла (сек). */
  sourceDuration: number
  trimStart: number
  trimEnd: number
  /** Позиция начала клипа на общем таймлайне (сек). */
  start: number
  color?: ClipColor
  audio?: ClipAudio
}

export interface TimelineTrack {
  id: string
  kind: TrackKind
  clips: TimelineClip[]
}

export interface Timeline {
  version: number
  duration: number
  tracks: TimelineTrack[]
}

export type ProjectStatus = 'draft' | 'exporting' | 'exported'

export interface EditorProject {
  id: string
  userId: string
  title: string
  sourceVideoId?: string | null
  isRemix: boolean
  timeline: Timeline
  status: ProjectStatus
  lastRenderJobId?: string | null
  resultVideoId?: string | null
  createdAt: string
  updatedAt: string
}

export type RenderStatus = 'queued' | 'processing' | 'completed' | 'failed'

export interface RenderJob {
  id: string
  videoId?: string | null
  userId: string
  status: RenderStatus
  progress: number
  outputUrl?: string
  error?: string
  createdAt: string
  updatedAt: string
}

export interface ExportSettings {
  title: string
  description: string
  category: Category
  visibility: Visibility
  tags: string[]
  allowRemix: boolean
  width: number
  height: number
  fps: number
  format: 'mp4' | 'webm'
  quality: 'low' | 'medium' | 'high'
}

export interface Comment {
  id: string
  videoId: string
  authorId: string
  text: string
  likes: number
  createdAt: string
}

export interface Subscription {
  id: string
  subscriberId: string
  channelId: string
  subscribedAt: string
}

export interface StatsPoint {
  date: string
  views: number
  subscribers: number
  likes: number
  dislikes: number
}

export interface ChannelStats {
  channelId: string
  points: StatsPoint[]
  subscribedRecently: string[]
  unsubscribedRecently: string[]
}

export interface PlatformStats {
  totalUsers: number
  totalChannels: number
  totalVideos: number
  totalViews: number
  totalComments: number
  dailyActive: { date: string; count: number }[]
}
