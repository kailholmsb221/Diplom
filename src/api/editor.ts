import { api } from './client'
import type { EditorProject, ExportSettings, RenderJob, Timeline } from '@/shared/types'

interface CreateProjectInput {
  title?: string
  sourceVideoId?: string | null
  isRemix?: boolean
}

export async function createProject(input: CreateProjectInput): Promise<EditorProject> {
  return api<EditorProject>('/api/editor/projects', { method: 'POST', body: input })
}

export async function listProjects(): Promise<EditorProject[]> {
  return api<EditorProject[]>('/api/editor/projects')
}

export async function getProject(id: string): Promise<EditorProject> {
  return api<EditorProject>(`/api/editor/projects/${id}`)
}

export async function saveProject(
  id: string,
  patch: { title?: string; timeline?: Timeline },
): Promise<EditorProject> {
  return api<EditorProject>(`/api/editor/projects/${id}`, { method: 'PATCH', body: patch })
}

export async function deleteProject(id: string): Promise<void> {
  await api(`/api/editor/projects/${id}`, { method: 'DELETE' })
}

export interface ExportResponse {
  job: RenderJob
  resultVideoId: string
}

export async function exportProject(id: string, settings: ExportSettings): Promise<ExportResponse> {
  return api<ExportResponse>(`/api/editor/projects/${id}/export`, { method: 'POST', body: settings })
}

export async function getExportJob(projectId: string, jobId: string): Promise<RenderJob> {
  return api<RenderJob>(`/api/editor/projects/${projectId}/export/${jobId}`)
}

/** Разрешить/запретить «Создать свою версию» (вызывает автор видео). */
export async function setRemixPermission(videoId: string, allow: boolean): Promise<void> {
  await api(`/api/videos/${videoId}/remix-permission`, { method: 'POST', body: { allow } })
}
