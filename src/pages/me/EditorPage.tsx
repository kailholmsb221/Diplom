import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowLeft,
  Download,
  Redo2,
  Save,
  Scissors,
  Undo2,
  ZoomIn,
  ZoomOut,
} from 'lucide-react'
import { Button } from '@/shared/ui/Button'
import { Loader, ErrorState } from '@/shared/ui/states'
import { toast } from '@/shared/ui/toast'
import { getProject, saveProject } from '@/api/editor'
import type { EditorProject, Timeline } from '@/shared/types'
import { MediaLibrary } from '@/features/editor/MediaLibrary'
import { PreviewPane } from '@/features/editor/PreviewPane'
import { TimelineView } from '@/features/editor/TimelineView'
import { PropertiesPanel } from '@/features/editor/PropertiesPanel'
import { ExportDialog } from '@/features/editor/ExportDialog'
import { useEditorEngine, audioTrack } from '@/features/editor/useEditorEngine'
import { probeDuration } from '@/features/editor/mediaProbe'

export function EditorPage() {
  const { projectId = '' } = useParams()
  const projectQ = useQuery({
    queryKey: ['editor-project', projectId],
    queryFn: () => getProject(projectId),
    enabled: !!projectId,
    refetchOnWindowFocus: false,
  })

  if (projectQ.isLoading) return <Loader label="Загрузка проекта…" />
  if (projectQ.isError || !projectQ.data)
    return <ErrorState title="Проект не найден" message="Возможно, он удалён или у вас нет доступа." />

  return <Workspace key={projectQ.data.id} project={projectQ.data} />
}

function normalizeTimeline(t: Timeline | undefined): Timeline {
  if (t && Array.isArray(t.tracks) && t.tracks.length > 0) return t
  return {
    version: 1,
    duration: 1,
    tracks: [
      { id: 'video-1', kind: 'video', clips: [] },
      { id: 'audio-1', kind: 'audio', clips: [] },
    ],
  }
}

function Workspace({ project }: { project: EditorProject }) {
  const engine = useEditorEngine(normalizeTimeline(project.timeline))
  const { state, dispatch, selectedClip, canUndo, canRedo } = engine
  const [title, setTitle] = useState(project.title)
  const [playing, setPlaying] = useState(false)
  const [showExport, setShowExport] = useState(false)
  const [saving, setSaving] = useState(false)
  const lastSavedRef = useRef<string>('')

  const isAudioSelected = !!selectedClip && (audioTrack(state.timeline)?.clips.some((c) => c.id === selectedClip.id) ?? false)

  // Авто-восстановление длительности клипов с повреждёнными данными (старые проекты,
  // где trim/sourceDuration сохранились как NaN). Узнаём реальную длину по метаданным
  // файла и чиним — без этого таймлайн такого клипа неисправен (длительность = NaN).
  const probedRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    const vt = state.timeline.tracks.find((tr) => tr.kind === 'video')
    if (!vt) return
    for (const c of vt.clips) {
      const d = c.trimEnd - c.trimStart
      const broken = !Number.isFinite(d) || d <= 0 || !Number.isFinite(c.sourceDuration) || c.sourceDuration <= 0
      if (broken && c.sourceUrl && !probedRef.current.has(c.id)) {
        probedRef.current.add(c.id)
        probeDuration(c.sourceUrl).then((dur) => {
          if (dur > 0) dispatch({ type: 'repairClip', id: c.id, sourceDuration: dur, trimEnd: dur })
        })
      }
    }
  }, [state.timeline, dispatch])

  const doSave = useCallback(
    async (silent: boolean) => {
      setSaving(true)
      try {
        await saveProject(project.id, { title, timeline: state.timeline })
        dispatch({ type: 'markSaved' })
        lastSavedRef.current = JSON.stringify(state.timeline)
        if (!silent) toast('Черновик сохранён')
      } catch {
        if (!silent) toast('Не удалось сохранить черновик', 'error')
      } finally {
        setSaving(false)
      }
    },
    [project.id, title, state.timeline, dispatch],
  )

  // Автосохранение: через 2.5 c после последнего изменения таймлайна.
  useEffect(() => {
    if (!state.dirty) return
    const snapshot = JSON.stringify(state.timeline)
    if (snapshot === lastSavedRef.current) return
    const t = window.setTimeout(() => doSave(true), 2500)
    return () => window.clearTimeout(t)
  }, [state.timeline, state.dirty, doSave])

  // Горячие клавиши.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
      if (e.code === 'Space') {
        e.preventDefault()
        setPlaying((p) => !p)
      } else if (e.key === 's' && !e.ctrlKey && !e.metaKey) {
        dispatch({ type: 'split' })
      } else if ((e.key === 'Delete' || e.key === 'Backspace') && selectedClip) {
        dispatch({ type: 'delete', id: selectedClip.id })
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z' && !e.shiftKey) {
        e.preventDefault()
        dispatch({ type: 'undo' })
      } else if ((e.ctrlKey || e.metaKey) && (e.key.toLowerCase() === 'y' || (e.shiftKey && e.key.toLowerCase() === 'z'))) {
        e.preventDefault()
        dispatch({ type: 'redo' })
      } else if (e.key === '+' || e.key === '=') {
        dispatch({ type: 'setZoom', pxPerSec: state.pxPerSec * 1.25 })
      } else if (e.key === '-') {
        dispatch({ type: 'setZoom', pxPerSec: state.pxPerSec / 1.25 })
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [dispatch, selectedClip, state.pxPerSec])

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem)] -mx-4 -my-4">
      {/* Тулбар */}
      <div className="flex items-center gap-2 px-4 h-12 border-b border-border bg-surface shrink-0">
        <Link to="/me/videos" className="text-muted hover:text-text" title="Назад">
          <ArrowLeft className="w-5 h-5" />
        </Link>
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          className="bg-transparent text-text font-medium px-2 h-8 rounded-md hover:bg-elevated focus:bg-elevated outline-none min-w-0 max-w-[220px]"
        />
        <div className="h-5 w-px bg-border mx-1" />
        <Button size="sm" variant="ghost" disabled={!canUndo} onClick={() => dispatch({ type: 'undo' })} title="Ctrl+Z">
          <Undo2 className="w-4 h-4" />
        </Button>
        <Button size="sm" variant="ghost" disabled={!canRedo} onClick={() => dispatch({ type: 'redo' })} title="Ctrl+Y">
          <Redo2 className="w-4 h-4" />
        </Button>
        <Button size="sm" variant="ghost" onClick={() => dispatch({ type: 'split' })} title="Разрезать (S)">
          <Scissors className="w-4 h-4" />
        </Button>

        <div className="ml-auto flex items-center gap-2">
          <Button size="sm" variant="ghost" onClick={() => dispatch({ type: 'setZoom', pxPerSec: state.pxPerSec / 1.25 })}>
            <ZoomOut className="w-4 h-4" />
          </Button>
          <Button size="sm" variant="ghost" onClick={() => dispatch({ type: 'setZoom', pxPerSec: state.pxPerSec * 1.25 })}>
            <ZoomIn className="w-4 h-4" />
          </Button>
          <Button size="sm" variant="subtle" disabled={saving} onClick={() => doSave(false)}>
            <Save className="w-4 h-4" /> {state.dirty ? 'Сохранить' : 'Сохранено'}
          </Button>
          <Button size="sm" variant="primary" onClick={() => setShowExport(true)}>
            <Download className="w-4 h-4" /> Экспорт
          </Button>
        </div>
      </div>

      {/* Тело: медиатека | превью | свойства */}
      <div className="flex flex-1 min-h-0">
        <aside className="w-64 shrink-0 border-r border-border bg-surface overflow-y-auto hidden md:block">
          <MediaLibrary dispatch={dispatch} />
        </aside>

        <main className="flex-1 min-w-0 flex flex-col">
          <div className="flex-1 min-h-0 overflow-auto p-4 grid place-items-center bg-bg">
            <div className="w-full max-w-3xl">
              <PreviewPane
                timeline={state.timeline}
                playhead={state.playhead}
                playing={playing}
                onTime={(t) => dispatch({ type: 'setPlayhead', time: t })}
                onTogglePlay={() => setPlaying((p) => !p)}
              />
            </div>
          </div>
          <div className="border-t border-border p-3 bg-surface">
            <TimelineView
              timeline={state.timeline}
              playhead={state.playhead}
              pxPerSec={state.pxPerSec}
              selectedClipId={state.selectedClipId}
              dispatch={dispatch}
              onScrub={() => setPlaying(false)}
            />
          </div>
        </main>

        <aside className="w-72 shrink-0 border-l border-border bg-surface overflow-y-auto hidden lg:block">
          <PropertiesPanel clip={selectedClip} isAudio={isAudioSelected} dispatch={dispatch} />
        </aside>
      </div>

      {showExport && (
        <ExportDialog
          projectId={project.id}
          defaultTitle={title}
          onClose={() => setShowExport(false)}
          onBeforeExport={() => doSave(true)}
        />
      )}
    </div>
  )
}
