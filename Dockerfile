FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/webhook-service ./cmd/webhook-service

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /out/webhook-service /usr/local/bin/webhook-service
COPY config.example.yaml /etc/webhook/config.yaml

ENV CONFIG_FILE=/etc/webhook/config.yaml

EXPOSE 8080

ENTRYPOINT ["webhook-service", "-config", "/etc/webhook/config.yaml"]
