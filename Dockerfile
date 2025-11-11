FROM golang:1.22 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/webhook-service ./cmd/webhook-service

FROM alpine:3.20
RUN addgroup -S webhook && adduser -S webhook -G webhook

WORKDIR /home/webhook
COPY --from=builder /out/webhook-service /usr/local/bin/webhook-service
COPY config.sample.yaml /etc/webhook/config.yaml

USER webhook
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/webhook-service", "--config", "/etc/webhook/config.yaml"]
