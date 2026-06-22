#!/usr/bin/env python3
"""
Worker для транскрипции видео и перевода субтитров.

Вызов из Go:
    python3 transcribe.py <video_id> <video_file_path>

Среда:
    DATABASE_URL     — строка подключения к Postgres
    UPLOADS_DIR      — корень папки uploads (для VTT)
    WHISPER_MODEL    — размер модели faster-whisper (tiny / base / small / medium / large-v3)
                       по умолчанию "small"
    WHISPER_COMPUTE  — compute_type ("int8" по умолчанию — CPU-friendly)

Логика:
    1. faster-whisper с auto-detect языка → сегменты и определённый language.
    2. Сохраняем оригинальный transcript (status=COMPLETED).
    3. Переводим сегменты Argos'ом на 2 других языка из набора {ru, kk, en}.
       — Argos скачивает модели один раз и кладёт в ~/.local/share/argos-translate.
    4. Генерим VTT-файлы в uploads/SUBTITLES/<videoId>_<lang>.vtt.
    5. Создаём авто-главы (chapters) на видео — раз в 30–60 секунд.
    6. Обновляем сводный status видео на COMPLETED.

Ошибки не валят весь процесс: если перевод не получился, оригинал остаётся
COMPLETED, переводы помечаются FAILED — пользователь сможет перезапустить
их через POST /api/videos/{id}/translate.
"""
from __future__ import annotations

import json
import os
import re
import sys
import traceback
from datetime import datetime, timezone
from pathlib import Path

import psycopg

SUPPORTED_LANGS = ("ru", "kk", "en")
LANG_NAMES = {"ru": "Russian", "kk": "Kazakh", "en": "English"}


# ---------- DB helpers ----------

def db_connect():
    dsn = os.environ["DATABASE_URL"]
    # psycopg3 принимает libpq DSN или connection string в виде URL.
    return psycopg.connect(dsn, autocommit=True)


def set_status(conn, video_id: str, status: str, error: str = "") -> None:
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE videos SET transcript_status=%s, transcript_error=%s WHERE id=%s",
            (status, error, video_id),
        )


def upsert_transcript(
    conn,
    video_id: str,
    language: str,
    *,
    is_original: bool,
    status: str,
    full_text: str = "",
    segments: list | None = None,
    vtt_path: str = "",
    error: str = "",
) -> None:
    segments_json = json.dumps(segments or [], ensure_ascii=False)
    with conn.cursor() as cur:
        cur.execute(
            """
            INSERT INTO transcripts(video_id, language, is_original, status, full_text,
                                     segments, vtt_path, error, updated_at)
            VALUES (%s, %s, %s, %s, %s, %s::jsonb, %s, %s, NOW())
            ON CONFLICT (video_id, language) DO UPDATE SET
                is_original = EXCLUDED.is_original,
                status      = EXCLUDED.status,
                full_text   = EXCLUDED.full_text,
                segments    = EXCLUDED.segments,
                vtt_path    = EXCLUDED.vtt_path,
                error       = EXCLUDED.error,
                updated_at  = NOW()
            """,
            (video_id, language, is_original, status, full_text,
             segments_json, vtt_path, error),
        )


def set_original_language(conn, video_id: str, language: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE videos SET original_language=%s WHERE id=%s",
            (language, video_id),
        )


def set_chapters(conn, video_id: str, chapters: list) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE videos SET chapters=%s::jsonb WHERE id=%s",
            (json.dumps(chapters, ensure_ascii=False), video_id),
        )


# ---------- VTT ----------

def format_vtt_time(seconds: float) -> str:
    if seconds < 0:
        seconds = 0
    hours = int(seconds // 3600)
    minutes = int((seconds % 3600) // 60)
    secs = seconds - hours * 3600 - minutes * 60
    return f"{hours:02d}:{minutes:02d}:{secs:06.3f}".replace(".", ".")


def write_vtt(path: Path, segments: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = ["WEBVTT", ""]
    for i, seg in enumerate(segments, start=1):
        text = seg["text"].strip()
        if not text:
            continue
        lines.append(str(i))
        lines.append(f"{format_vtt_time(seg['start'])} --> {format_vtt_time(seg['end'])}")
        lines.append(text)
        lines.append("")
    path.write_text("\n".join(lines), encoding="utf-8")


# ---------- Whisper ----------

def transcribe_video(video_path: str) -> tuple[str, list[dict]]:
    """Запускает faster-whisper и возвращает (lang, segments)."""
    from faster_whisper import WhisperModel  # импорт только при наличии

    model_size = os.environ.get("WHISPER_MODEL", "small")
    compute_type = os.environ.get("WHISPER_COMPUTE", "int8")
    print(f"[whisper] loading model size={model_size} compute={compute_type}", flush=True)
    model = WhisperModel(model_size, compute_type=compute_type, device="cpu")

    print("[whisper] transcribing", video_path, flush=True)
    seg_iter, info = model.transcribe(
        video_path,
        beam_size=1,
        vad_filter=True,
        vad_parameters={"min_silence_duration_ms": 500},
    )
    lang = (info.language or "").lower()
    segments: list[dict] = []
    for s in seg_iter:
        segments.append({
            "start": float(s.start or 0.0),
            "end": float(s.end or 0.0),
            "text": (s.text or "").strip(),
        })
        # Поток сегментов — печатаем прогресс изредка.
        if len(segments) % 25 == 0:
            print(f"[whisper] segments so far: {len(segments)}", flush=True)
    print(f"[whisper] done lang={lang} segments={len(segments)}", flush=True)
    return lang, segments


def normalize_lang(lang: str) -> str:
    lang = (lang or "").lower()
    if lang.startswith("kk") or lang == "kazakh":
        return "kk"
    if lang.startswith("ru") or lang == "russian":
        return "ru"
    if lang.startswith("en") or lang == "english":
        return "en"
    return lang


# ---------- Argos Translate ----------

def install_argos_pair(src: str, tgt: str) -> bool:
    """Гарантирует, что пакет перевода src→tgt установлен. Возвращает True/False."""
    if src == tgt:
        return True
    try:
        import argostranslate.package  # type: ignore
        import argostranslate.translate  # type: ignore
    except Exception as e:
        print(f"[argos] not available: {e}", flush=True)
        return False

    installed = {l.code for l in argostranslate.translate.get_installed_languages()}
    if src in installed and tgt in installed:
        # уже есть оба языка — пара тоже должна быть
        return True

    try:
        argostranslate.package.update_package_index()
        available = argostranslate.package.get_available_packages()
        match = next((p for p in available if p.from_code == src and p.to_code == tgt), None)
        if not match:
            print(f"[argos] no package for {src}->{tgt}", flush=True)
            return False
        path = match.download()
        argostranslate.package.install_from_path(path)
        return True
    except Exception as e:
        print(f"[argos] install failed {src}->{tgt}: {e}", flush=True)
        return False


def translate_segments(segments: list[dict], src: str, tgt: str) -> list[dict] | None:
    if src == tgt:
        return [dict(s) for s in segments]
    if not install_argos_pair(src, tgt):
        return None
    try:
        import argostranslate.translate  # type: ignore
        languages = argostranslate.translate.get_installed_languages()
        from_lang = next((l for l in languages if l.code == src), None)
        to_lang = next((l for l in languages if l.code == tgt), None)
        if not from_lang or not to_lang:
            return None
        translation = from_lang.get_translation(to_lang)
        if translation is None:
            return None
        out: list[dict] = []
        for s in segments:
            translated = translation.translate(s["text"]) if s["text"].strip() else ""
            out.append({"start": s["start"], "end": s["end"], "text": translated})
        return out
    except Exception as e:
        print(f"[argos] translate failed {src}->{tgt}: {e}", flush=True)
        return None


# ---------- Chapters ----------

CHAPTER_GAP_SEC = 45.0
CHAPTER_MAX_WORDS = 7


def truncate_title(text: str) -> str:
    text = re.sub(r"\s+", " ", text).strip().strip(".,;:!?-—")
    words = text.split(" ")
    if len(words) > CHAPTER_MAX_WORDS:
        text = " ".join(words[:CHAPTER_MAX_WORDS]) + "…"
    return text or "…"


def build_chapters(segments: list[dict]) -> list[dict]:
    if not segments:
        return []
    chapters: list[dict] = []
    last_start = -CHAPTER_GAP_SEC
    for s in segments:
        if s["start"] - last_start < CHAPTER_GAP_SEC and chapters:
            continue
        title = truncate_title(s["text"])
        chapters.append({"start": round(float(s["start"]), 2), "title": title})
        last_start = s["start"]
        if len(chapters) >= 12:
            break
    return chapters


# ---------- Main ----------

def run(video_id: str, video_path: str) -> int:
    conn = db_connect()
    set_status(conn, video_id, "PROCESSING", "")
    for lang in SUPPORTED_LANGS:
        upsert_transcript(conn, video_id, lang,
                          is_original=False, status="PROCESSING")

    try:
        detected, segments = transcribe_video(video_path)
    except Exception as e:
        traceback.print_exc()
        msg = f"transcribe failed: {e}"
        set_status(conn, video_id, "FAILED", msg)
        for lang in SUPPORTED_LANGS:
            upsert_transcript(conn, video_id, lang, is_original=False,
                              status="FAILED", error=msg)
        return 1

    src = normalize_lang(detected)
    if src not in SUPPORTED_LANGS:
        # Whisper детектировал не наш язык — считаем оригиналом английский,
        # это самый частый fallback (или язык, наиболее близкий).
        print(f"[lang] {detected!r} not in supported, defaulting to 'en'", flush=True)
        src = "en"

    set_original_language(conn, video_id, src)

    uploads_dir = Path(os.environ.get("UPLOADS_DIR", "./uploads")).resolve()
    sub_dir = uploads_dir / "SUBTITLES"
    sub_dir.mkdir(parents=True, exist_ok=True)

    # ---- Сохраняем оригинал ----
    full_text = " ".join(s["text"] for s in segments).strip()
    vtt_path_rel_orig = f"SUBTITLES/{video_id}_{src}.vtt"
    write_vtt(sub_dir / f"{video_id}_{src}.vtt", segments)
    upsert_transcript(conn, video_id, src,
                      is_original=True, status="COMPLETED",
                      full_text=full_text, segments=segments,
                      vtt_path=vtt_path_rel_orig)

    # Главы — на основе оригинальных сегментов.
    chapters = build_chapters(segments)
    set_chapters(conn, video_id, chapters)

    set_status(conn, video_id, "TRANSLATING", "")

    # ---- Переводы ----
    failed = []
    for tgt in SUPPORTED_LANGS:
        if tgt == src:
            continue
        translated = translate_segments(segments, src, tgt)
        if translated is None:
            failed.append(tgt)
            upsert_transcript(conn, video_id, tgt,
                              is_original=False, status="FAILED",
                              error=f"argos translate {src}->{tgt} failed")
            continue
        tr_text = " ".join(t["text"] for t in translated).strip()
        vtt_rel = f"SUBTITLES/{video_id}_{tgt}.vtt"
        write_vtt(sub_dir / f"{video_id}_{tgt}.vtt", translated)
        upsert_transcript(conn, video_id, tgt,
                          is_original=False, status="COMPLETED",
                          full_text=tr_text, segments=translated,
                          vtt_path=vtt_rel)

    overall = "COMPLETED" if not failed else "COMPLETED"
    err_msg = "" if not failed else f"translation failed for: {', '.join(failed)}"
    set_status(conn, video_id, overall, err_msg)
    conn.close()
    print(f"[done] {video_id} src={src} failed_langs={failed}", flush=True)
    return 0


def translate_only(video_id: str) -> int:
    """Только перевод — оригинал должен быть уже сохранён."""
    conn = db_connect()
    with conn.cursor() as cur:
        cur.execute("""SELECT original_language FROM videos WHERE id=%s""", (video_id,))
        row = cur.fetchone()
        if not row or not row[0]:
            print("[translate] original language unknown", flush=True)
            return 2
        src = row[0]
        cur.execute("""SELECT segments FROM transcripts WHERE video_id=%s AND language=%s""",
                    (video_id, src))
        seg_row = cur.fetchone()
        if not seg_row:
            print("[translate] original segments missing", flush=True)
            return 3
        segments = seg_row[0]

    set_status(conn, video_id, "TRANSLATING", "")
    uploads_dir = Path(os.environ.get("UPLOADS_DIR", "./uploads")).resolve()
    sub_dir = uploads_dir / "SUBTITLES"

    for tgt in SUPPORTED_LANGS:
        if tgt == src:
            continue
        upsert_transcript(conn, video_id, tgt, is_original=False, status="TRANSLATING")
        translated = translate_segments(segments, src, tgt)
        if translated is None:
            upsert_transcript(conn, video_id, tgt, is_original=False,
                              status="FAILED",
                              error=f"argos translate {src}->{tgt} failed")
            continue
        tr_text = " ".join(t["text"] for t in translated).strip()
        vtt_rel = f"SUBTITLES/{video_id}_{tgt}.vtt"
        write_vtt(sub_dir / f"{video_id}_{tgt}.vtt", translated)
        upsert_transcript(conn, video_id, tgt, is_original=False,
                          status="COMPLETED", full_text=tr_text,
                          segments=translated, vtt_path=vtt_rel)

    set_status(conn, video_id, "COMPLETED", "")
    conn.close()
    return 0


def main() -> int:
    if len(sys.argv) >= 3 and sys.argv[1] == "translate":
        return translate_only(sys.argv[2])
    if len(sys.argv) < 3:
        print("usage: transcribe.py <video_id> <video_path> | translate <video_id>", flush=True)
        return 64
    return run(sys.argv[1], sys.argv[2])


if __name__ == "__main__":
    sys.exit(main())
