FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api    ./cmd/api
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM alpine:3.19 AS api
RUN apk add --no-cache curl
COPY --from=builder /out/api /api
ENTRYPOINT ["/api"]

FROM alpine:3.19 AS worker
# deno: yt-dlp needs a JS runtime to solve YouTube's bot-check challenge —
# without one, real downloads fail with "Sign in to confirm you're not a
# bot" (found live in CI; yt-dlp's own warning names deno as the runtime it
# looks for by default).
RUN apk add --no-cache ffmpeg python3 py3-pip deno && \
    pip install --break-system-packages --no-cache-dir yt-dlp
# yt-dlp merges separately-downloaded video/audio formats into a container
# that doesn't always match the extension in ytdlp.go's `-o` destPath (e.g.
# ending up as video.mp4.webm instead of video.mp4), which breaks the
# downstream ffprobe step that expects the exact requested path. Forcing the
# merge output format to mp4 system-wide keeps the downloaded file at the
# exact path internal/video/ytdlp.go requests, with no change to that file.
RUN echo "--merge-output-format mp4" > /etc/yt-dlp.conf
COPY --from=builder /out/worker /worker
ENTRYPOINT ["/worker"]

FROM node:20-alpine AS web-builder
WORKDIR /src/web
COPY web/ .
RUN npm ci
RUN npm run build

FROM nginx:1.27-alpine AS web
COPY --from=web-builder /src/web/dist /usr/share/nginx/html
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
