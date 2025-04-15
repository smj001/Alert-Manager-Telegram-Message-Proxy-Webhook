FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /telegram-webhook

FROM alpine:latest

WORKDIR /app

COPY --from=builder /telegram-webhook /app/telegram-webhook

EXPOSE 8080

CMD ["/app/telegram-webhook"] 