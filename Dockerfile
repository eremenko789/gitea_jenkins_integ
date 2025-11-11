# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Копируем go mod файлы
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/bin/gitea-jenkins-integ ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Копируем бинарник из builder stage
COPY --from=builder /app/bin/gitea-jenkins-integ .

# Копируем конфиг (если нужен дефолтный)
COPY config.yaml.example config.yaml.example

EXPOSE 8080

CMD ["./gitea-jenkins-integ"]
