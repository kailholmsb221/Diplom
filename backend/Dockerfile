# syntax=docker/dockerfile:1.6

# --- build stage (Go) -----------------------------------------------------
FROM golang:1.22-alpine AS build

WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/videohub-server ./cmd/server

# --- runtime stage --------------------------------------------------------
# python:3.11-slim — здесь же запускается Python-worker для транскрипции
# (faster-whisper + Argos Translate). Если transcription не нужна, можно
# вернуться на alpine и удалить установку pip-пакетов.
FROM python:3.11-slim

# ffmpeg нужен и faster-whisper, и для возможной конвертации форматов.
# libgomp1 требуется CTranslate2 (backend faster-whisper) на CPU.
RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg \
        ca-certificates \
        tzdata \
        libgomp1 \
        curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1001 app

WORKDIR /app

# Python-зависимости worker'а. Можно выключить, передав --build-arg
# SKIP_WORKER=1 — в этом случае worker недоступен, транскрипция помечается
# FAILED, остальная функциональность работает как обычно.
ARG SKIP_WORKER=0
COPY worker/requirements.txt /app/worker/requirements.txt
RUN if [ "$SKIP_WORKER" != "1" ]; then \
        pip install --upgrade pip \
        && pip install -r /app/worker/requirements.txt; \
    else \
        echo "skipping transcription worker dependencies"; \
    fi

COPY worker /app/worker
COPY --from=build /out/videohub-server /app/videohub-server
COPY migrations /app/migrations

RUN mkdir -p /app/uploads/MP4 /app/uploads/PNG /app/uploads/ADS /app/uploads/SUBTITLES \
    && chown -R app:app /app

USER app

EXPOSE 8080

ENV HTTP_ADDR=:8080 \
    UPLOADS_DIR=/app/uploads \
    PYTHON_BIN=python3 \
    WHISPER_WORKER=/app/worker/transcribe.py \
    WHISPER_MODEL=small \
    WHISPER_COMPUTE=int8 \
    RENDER_WORKER=/app/worker/render.py \
    FFMPEG_BIN=ffmpeg \
    FFPROBE_BIN=ffprobe

ENTRYPOINT ["/app/videohub-server"]
