/**
 * Узнаёт длительность медиафайла по URL через скрытый media-элемент
 * (без перекодирования). Для аудиофайлов передавайте kind='audio'.
 */
export function probeDuration(url: string, kind: 'video' | 'audio' = 'video'): Promise<number> {
  return new Promise((resolve) => {
    const v = document.createElement(kind)
    v.preload = 'metadata'
    v.muted = true
    const done = (d: number) => {
      v.removeAttribute('src')
      v.load()
      resolve(Number.isFinite(d) && d > 0 ? d : 0)
    }
    v.onloadedmetadata = () => done(v.duration)
    v.onerror = () => done(0)
    v.src = url
  })
}
