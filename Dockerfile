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
RUN apk add --no-cache ffmpeg yt-dlp
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
