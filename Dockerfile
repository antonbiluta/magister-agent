FROM golang:1.23-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go config.yaml ./
RUN go build -o agent main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/agent .
COPY --from=builder /app/config.yaml .
ENTRYPOINT ["./agent"]