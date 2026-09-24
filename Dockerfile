# Сборка
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy
RUN go mod download
RUN CGO_ENABLED=0 go build -o /app/server ./cmd/server

# Финальный образ
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server /server
COPY --from=builder /app/web ./web
COPY --from=builder /app/config ./config  
COPY --from=builder /app/content ./content
EXPOSE 8080
CMD ["/server"]