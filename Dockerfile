FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY . .

RUN go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /telegram-webhook ./cmd/server

FROM alpine:3.23

LABEL Name="telegram-webhook" \
    Version="1.0.0"

WORKDIR /app

COPY --from=builder /telegram-webhook /app/telegram-webhook

USER 65534:65534
EXPOSE 8080

CMD ["/app/telegram-webhook"]
