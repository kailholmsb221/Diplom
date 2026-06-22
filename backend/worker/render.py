#!/usr/bin/env python3
"""
Worker серверного рендера (экспорта) видео из встроенного редактора.

Вызов из Go (render.Spawner):
    python3 render.py <render_job_id>

Среда:
    DATABASE_URL    — строка подключения к Postgres
    UPLOADS_DIR     — корень папки uploads (MP4/ PNG/ HLS/ TMP/)
    PUBLIC_BASE_URL — базовый URL для ссылок (…/uploads/MP4/<file>)
    FFMPEG_BIN      — путь к ffmpeg  (по умолчанию "ffmpeg")
    FFPROBE_BIN     — путь к ffprobe (по умолчанию "ffprobe")

Логика (НЕразрушающая — оригиналы не трогаются):
    1. Читаем render_jobs по id: manifest (EditManifest), video_id (новое видео).
    2. Для каждого видеоклипа: -ss/-to обрезка → фильтры цветокоррекции (eq/hue/
       curves/unsharp/colortemperature/opacity) + громкость/fade → нормализованный
       сегмент seg_i.mp4 (одинаковые кодеки/разрешение/fps — для надёжного concat).
    3. Склейка сегментов concat-демультиплексором → base.mp4.
    4. Если есть доп. аудиодорожки — микшируем (adelay + amix) → final.
    5. Генерим обложку (кадр на ~1 c) → uploads/PNG.
    6. Обновляем videos (video_url, thumbnail_url, duration_sec, visibility),
       render_jobs (completed + output_url) и editor_projects (exported).

Ошибки помечают задачу failed и пишут текст в render_jobs.error — HTTP-запрос
экспорта при этом давно завершён (рендер идёт в фоне).
"""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import traceback
import uuid
from pathlib import Path
from urllib.parse import urlparse

import psycopg

FFMPEG = os.environ.get("FFMPEG_BIN", "ffmpeg")
FFPROBE = os.environ.get("FFPROBE_BIN", "ffprobe")
UPLOADS_DIR = Path(os.environ.get("UPLOADS_DIR", "./uploads")).resolve()
PUBLIC_BASE_URL = os.environ.get("PUBLIC_BASE_URL", "http://localhost:8080").rstrip("/")

CRF_BY_QUALITY = {"low": "30", "medium": "24", "high": "19"}


# ----------------------------- DB helpers -----------------------------

def db_connect():
    return psycopg.connect(os.environ["DATABASE_URL"], autocommit=True)


def set_progress(conn, job_id: str, progress: int) -> None:
    progress = max(0, min(100, int(progress)))
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE render_jobs SET progress=%s, status='processing', updated_at=NOW() "
            "WHERE id=%s AND status IN ('queued','processing')",
            (progress, job_id),
        )


def mark_failed(conn, job_id: str, msg: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE render_jobs SET status='failed', error=%s, updated_at=NOW() WHERE id=%s",
            (msg[:2000], job_id),
        )


def mark_completed(conn, job_id: str, output_url: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE render_jobs SET status='completed', progress=100, output_url=%s, "
            "error='', updated_at=NOW() WHERE id=%s",
            (output_url, job_id),
        )


# ----------------------------- ffmpeg helpers -----------------------------

def local_path_from_url(url: str) -> Path:
    """Превращает публичный URL …/uploads/<rel> в путь на диске внутри UPLOADS_DIR."""
    marker = "/uploads/"
    idx = url.find(marker)
    rel = url[idx + len(marker):] if idx >= 0 else url
    rel = rel.lstrip("/")
    p = (UPLOADS_DIR / rel).resolve()
    # Защита от выхода за пределы uploads (path traversal).
    if not str(p).startswith(str(UPLOADS_DIR)):
        raise ValueError(f"source outside uploads: {url}")
    if not p.exists():
        raise FileNotFoundError(f"source file not found: {p}")
    return p


_probe_cache: dict[str, dict] = {}


def probe(path: Path) -> dict:
    key = str(path)
    if key in _probe_cache:
        return _probe_cache[key]
    out = subprocess.run(
        [FFPROBE, "-v", "error", "-print_format", "json",
         "-show_streams", "-show_format", str(path)],
        capture_output=True, text=True,
    )
    info = {"has_audio": False, "duration": 0.0}
    try:
        data = json.loads(out.stdout or "{}")
        for st in data.get("streams", []):
            if st.get("codec_type") == "audio":
                info["has_audio"] = True
        info["duration"] = float(data.get("format", {}).get("duration", 0) or 0)
    except Exception:
        pass
    _probe_cache[key] = info
    return info


def near(value: float, target: float, eps: float = 1e-3) -> bool:
    return abs(value - target) <= eps


def build_color_filter(color: dict | None) -> str:
    """Строит цепочку видеофильтров цветокоррекции. Нейтральные значения опускаются."""
    if not color:
        return ""
    parts: list[str] = []

    b = float(color.get("brightness", 0) or 0)
    c = float(color.get("contrast", 1) or 1)
    s = float(color.get("saturation", 1) or 1)
    g = float(color.get("exposure", 1) or 1)
    eq = []
    if not near(b, 0):
        eq.append(f"brightness={b:.4f}")
    if not near(c, 1):
        eq.append(f"contrast={c:.4f}")
    if not near(s, 1):
        eq.append(f"saturation={s:.4f}")
    if not near(g, 1) and g > 0:
        eq.append(f"gamma={g:.4f}")
    if eq:
        parts.append("eq=" + ":".join(eq))

    t = float(color.get("temperature", 0) or 0)
    if not near(t, 0):
        kelvin = max(1000.0, min(40000.0, 6500.0 + t * 35.0))
        parts.append(f"colortemperature=temperature={kelvin:.0f}")

    sh = float(color.get("shadows", 0) or 0)
    hl = float(color.get("highlights", 0) or 0)
    if not near(sh, 0) or not near(hl, 0):
        # Кривая: поднимаем/опускаем тени в x=0.25 и света в x=0.75.
        y_sh = max(0.0, min(1.0, 0.25 + sh * 0.2))
        y_hl = max(0.0, min(1.0, 0.75 + hl * 0.2))
        parts.append(f"curves=all='0/0 0.25/{y_sh:.3f} 0.75/{y_hl:.3f} 1/1'")

    h = float(color.get("hue", 0) or 0)
    if not near(h, 0):
        parts.append(f"hue=h={h:.2f}")

    sp = float(color.get("sharpness", 0) or 0)
    if sp > 1e-3:
        parts.append(f"unsharp=5:5:{sp:.3f}:5:5:0")

    o = float(color.get("opacity", 1) or 1)
    if o < 0.999:
        o = max(0.0, o)
        parts.append(f"colorchannelmixer=rr={o:.3f}:gg={o:.3f}:bb={o:.3f}")

    return ",".join(parts)


def build_audio_filter(audio: dict | None, dur: float) -> str:
    parts: list[str] = []
    vol = 1.0
    fin = 0.0
    fout = 0.0
    if audio:
        vol = float(audio.get("volume", 1) or 1)
        fin = float(audio.get("fadeIn", 0) or 0)
        fout = float(audio.get("fadeOut", 0) or 0)
        if audio.get("muted"):
            vol = 0.0
    if not near(vol, 1):
        parts.append(f"volume={vol:.4f}")
    if fin > 1e-3:
        parts.append(f"afade=t=in:st=0:d={fin:.3f}")
    if fout > 1e-3:
        st = max(0.0, dur - fout)
        parts.append(f"afade=t=out:st={st:.3f}:d={fout:.3f}")
    return ",".join(parts)


def render_segment(clip: dict, idx: int, w: int, h: int, fps: int, crf: str,
                   workdir: Path) -> Path:
    """Рендерит один нормализованный сегмент с обрезкой, цветом и звуком."""
    src = local_path_from_url(clip["sourceUrl"])
    ts = float(clip.get("trimStart", 0) or 0)
    te = float(clip.get("trimEnd", 0) or 0)
    dur = max(0.05, te - ts)
    info = probe(src)

    vf = (f"scale={w}:{h}:force_original_aspect_ratio=decrease,"
          f"pad={w}:{h}:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1,fps={fps},format=yuv420p")
    color_vf = build_color_filter(clip.get("color"))
    if color_vf:
        vf = color_vf + "," + vf

    out = workdir / f"seg_{idx:03d}.mp4"
    cmd = [FFMPEG, "-y", "-ss", f"{ts:.3f}", "-to", f"{te:.3f}", "-i", str(src)]

    has_audio = info["has_audio"]
    muted = bool(clip.get("audio", {}).get("muted")) if clip.get("audio") else False
    if not has_audio or muted:
        # Гарантируем аудиодорожку (тишина) — иначе concat ломается на разнородных входах.
        cmd += ["-f", "lavfi", "-t", f"{dur:.3f}",
                "-i", "anullsrc=channel_layout=stereo:sample_rate=48000"]
        amap = "1:a"
        af = ""
    else:
        amap = "0:a"
        af = build_audio_filter(clip.get("audio"), dur)

    cmd += ["-map", "0:v", "-map", amap, "-vf", vf]
    if af:
        cmd += ["-af", af]
    cmd += [
        "-c:v", "libx264", "-preset", "veryfast", "-crf", crf, "-pix_fmt", "yuv420p",
        "-c:a", "aac", "-b:a", "192k", "-ar", "48000", "-ac", "2",
        "-movflags", "+faststart", str(out),
    ]
    run_ffmpeg(cmd)
    return out


def run_ffmpeg(cmd: list[str]) -> None:
    print("ffmpeg:", " ".join(cmd), flush=True)
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode != 0:
        tail = (res.stderr or "")[-1500:]
        raise RuntimeError(f"ffmpeg failed ({res.returncode}): {tail}")


def concat_segments(segments: list[Path], workdir: Path, out: Path) -> None:
    listfile = workdir / "concat.txt"
    listfile.write_text("".join(f"file '{p.as_posix()}'\n" for p in segments), encoding="utf-8")
    run_ffmpeg([FFMPEG, "-y", "-f", "concat", "-safe", "0", "-i", str(listfile),
                "-c", "copy", "-movflags", "+faststart", str(out)])


def mix_audio_tracks(base: Path, audio_clips: list[dict], out: Path) -> None:
    """Подмешивает доп. аудиодорожки (музыка/озвучка) к звуку базового видео."""
    inputs = ["-i", str(base)]
    filters = []
    amix_labels = ["[0:a]"]
    for i, ac in enumerate(audio_clips, start=1):
        src = local_path_from_url(ac["sourceUrl"])
        ts = float(ac.get("trimStart", 0) or 0)
        te = float(ac.get("trimEnd", 0) or 0)
        delay_ms = int(max(0.0, float(ac.get("start", 0) or 0)) * 1000)
        vol = float(ac.get("volume", 1) or 1)
        fin = float(ac.get("fadeIn", 0) or 0)
        fout = float(ac.get("fadeOut", 0) or 0)
        dur = max(0.05, te - ts)
        inputs += ["-ss", f"{ts:.3f}", "-to", f"{te:.3f}", "-i", str(src)]
        chain = [f"volume={vol:.4f}"]
        if fin > 1e-3:
            chain.append(f"afade=t=in:st=0:d={fin:.3f}")
        if fout > 1e-3:
            chain.append(f"afade=t=out:st={max(0.0, dur - fout):.3f}:d={fout:.3f}")
        chain.append(f"adelay={delay_ms}|{delay_ms}")
        filters.append(f"[{i}:a]" + ",".join(chain) + f"[a{i}]")
        amix_labels.append(f"[a{i}]")
    n = len(amix_labels)
    filters.append("".join(amix_labels) +
                   f"amix=inputs={n}:duration=first:dropout_transition=0,dynaudnorm[aout]")
    cmd = [FFMPEG, "-y"] + inputs + [
        "-filter_complex", ";".join(filters),
        "-map", "0:v", "-map", "[aout]",
        "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-ar", "48000", "-ac", "2",
        "-movflags", "+faststart", str(out),
    ]
    run_ffmpeg(cmd)


def make_thumbnail(video: Path, out: Path) -> bool:
    try:
        run_ffmpeg([FFMPEG, "-y", "-ss", "1", "-i", str(video),
                    "-frames:v", "1", "-q:v", "3", str(out)])
        return out.exists()
    except Exception as e:
        print("thumbnail failed:", e, flush=True)
        return False


# ----------------------------- main -----------------------------

def main() -> int:
    if len(sys.argv) < 2:
        print("usage: render.py <job_id>", file=sys.stderr)
        return 2
    job_id = sys.argv[1]

    conn = db_connect()
    try:
        with conn.cursor() as cur:
            cur.execute("SELECT video_id, manifest FROM render_jobs WHERE id=%s", (job_id,))
            row = cur.fetchone()
        if not row:
            print(f"render job {job_id} not found", file=sys.stderr)
            return 1
        video_id, manifest = row[0], row[1]
        if isinstance(manifest, (str, bytes)):
            manifest = json.loads(manifest)

        clips = manifest.get("clips") or []
        if not clips:
            mark_failed(conn, job_id, "manifest has no clips")
            return 1

        w = int(manifest.get("width") or 0) or 1280
        h = int(manifest.get("height") or 0) or 720
        fps = int(manifest.get("fps") or 0) or 30
        crf = CRF_BY_QUALITY.get(manifest.get("quality") or "medium", "24")
        fmt = (manifest.get("format") or "mp4").lower()
        ext = "webm" if fmt == "webm" else "mp4"
        visibility = "public" if manifest.get("visibility") == "public" else "private"
        audio_clips = manifest.get("audioClips") or []

        set_progress(conn, job_id, 2)

        with tempfile.TemporaryDirectory(prefix="render_") as tmp:
            workdir = Path(tmp)
            segments: list[Path] = []
            for i, clip in enumerate(clips):
                seg = render_segment(clip, i, w, h, fps, crf, workdir)
                segments.append(seg)
                set_progress(conn, job_id, 5 + int(70 * (i + 1) / len(clips)))

            base = workdir / "base.mp4"
            if len(segments) == 1:
                base = segments[0]
            else:
                concat_segments(segments, workdir, base)
            set_progress(conn, job_id, 82)

            final_name = f"{uuid.uuid4()}.{ext}"
            final_path = UPLOADS_DIR / "MP4" / final_name
            final_path.parent.mkdir(parents=True, exist_ok=True)

            if audio_clips:
                mixed = workdir / f"mixed.mp4"
                mix_audio_tracks(base, audio_clips, mixed)
                base = mixed
            set_progress(conn, job_id, 90)

            # Финализация в нужный контейнер (mp4: копируем; webm: перекодируем).
            if ext == "webm":
                run_ffmpeg([FFMPEG, "-y", "-i", str(base),
                            "-c:v", "libvpx-vp9", "-crf", crf, "-b:v", "0",
                            "-c:a", "libopus", "-b:a", "160k", str(final_path)])
            else:
                run_ffmpeg([FFMPEG, "-y", "-i", str(base), "-c", "copy",
                            "-movflags", "+faststart", str(final_path)])
            set_progress(conn, job_id, 94)

            # Обложка.
            thumb_url = ""
            thumb_name = f"{uuid.uuid4()}.jpg"
            thumb_path = UPLOADS_DIR / "PNG" / thumb_name
            thumb_path.parent.mkdir(parents=True, exist_ok=True)
            if make_thumbnail(final_path, thumb_path):
                thumb_url = f"{PUBLIC_BASE_URL}/uploads/PNG/{thumb_name}"

            duration = int(round(probe(final_path)["duration"]))
            video_url = f"{PUBLIC_BASE_URL}/uploads/MP4/{final_name}"

        # Запись результата в БД (видео + проект + задача).
        with conn.cursor() as cur:
            if video_id:
                cur.execute(
                    "UPDATE videos SET video_url=%s, "
                    "thumbnail_url = CASE WHEN %s <> '' THEN %s ELSE thumbnail_url END, "
                    "duration_sec = CASE WHEN %s > 0 THEN %s ELSE duration_sec END, "
                    "visibility=%s WHERE id=%s",
                    (video_url, thumb_url, thumb_url, duration, duration, visibility, video_id),
                )
                cur.execute(
                    "UPDATE editor_projects SET status='exported', updated_at=NOW() "
                    "WHERE result_video_id=%s",
                    (video_id,),
                )
        mark_completed(conn, job_id, video_url)
        print(f"render {job_id}: completed → {video_url}", flush=True)
        return 0

    except Exception as e:  # noqa: BLE001
        traceback.print_exc()
        try:
            mark_failed(conn, job_id, str(e))
        except Exception:
            pass
        return 1
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
