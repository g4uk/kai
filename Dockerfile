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
COPY --from=builder /out/worker /worker
ENTRYPOINT ["/worker"]
