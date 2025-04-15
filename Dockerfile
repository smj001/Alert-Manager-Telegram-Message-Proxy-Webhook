FROM golang:1.21.3-alpine3.18 AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /telegram-webhook

FROM alpine:3.18

LABEL Name="telegram-webhook" \
    Version="1.0.0"

WORKDIR /app

COPY --from=builder /telegram-webhook /app/telegram-webhook

EXPOSE 8080

CMD ["/app/telegram-webhook"] 