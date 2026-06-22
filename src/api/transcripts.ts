import { api } from './client'
import type { SubtitlesResponse, TranscriptLanguage, TranscriptResponse } from '@/shared/types'

export function fetchTranscript(
  videoId: string,
  lang: TranscriptLanguage,
): Promise<TranscriptResponse> {
  return api<TranscriptResponse>(`/api/videos/${videoId}/transcript`, { query: { lang } })
}

export function fetchSubtitles(videoId: string): Promise<SubtitlesResponse> {
  return api<SubtitlesResponse>(`/api/videos/${videoId}/subtitles`)
}

export function rerunTranscription(videoId: string): Promise<{ status: string; message: string }> {
  return api(`/api/videos/${videoId}/transcribe`, { method: 'POST' })
}

export function rerunTranslation(videoId: string): Promise<{ status: string; message: string }> {
  return api(`/api/videos/${videoId}/translate`, { method: 'POST' })
}
