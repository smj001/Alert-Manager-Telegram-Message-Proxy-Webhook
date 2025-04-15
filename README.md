# Alert Manager Telegram Message Proxy Webhook

A production-ready webhook service that forwards messages to Telegram. This service acts as a proxy between your applications and Telegram, allowing you to send messages to specific chat IDs through a secure webhook.

## Features

- Secure webhook endpoint with API key authentication
- Support for text messages
- Support for media attachments (photos, videos, documents)
- Environment variable configuration
- Docker containerization
- Production-ready structure

## Prerequisites

- Go 1.21 or later
- Docker (optional, for containerization)
- A Telegram bot token (get it from [@BotFather](https://t.me/BotFather))

## Quick Start

1. Clone the repository:
   ```bash
   git clone https://github.com/smj/Alert-Manager-Telegram-Message-Proxy-Webhook.git
   cd Alert-Manager-Telegram-Message-Proxy-Webhook
   ```

2. Create a `.env` file:
   ```bash
   cp .env.example .env
   ```
   Edit the `.env` file with your credentials:
   ```
   TELEGRAM_BOT_TOKEN=your_bot_token_here
   WEBHOOK_API_KEY=your_api_key_here
   SERVER_PORT=8080
   ```

3. Run the application:
   ```bash
   make run
   ```

## Building

To build the application:
```bash
make build
```

The binary will be created in the `bin/` directory.

## Docker

Build the Docker image:
```bash
docker build -t telegram-webhook .
```

Run the container:
```bash
docker run -d \
  -p 8080:8080 \
  --env-file .env \
  telegram-webhook
```

## API Usage

Send a POST request to `/webhook` with the following JSON body:

```json
{
  "chat_id": 123456789,
  "message": "Hello, World!",
  "media_url": "https://example.com/image.jpg",
  "media_type": "photo"
}
```

### Headers

- `Authorization: Bearer your_api_key`

### Media Types

Supported media types:
- `photo`
- `video`
- `document`

## Testing

Run the test suite:
```bash
make test
```

## Project Structure

```
.
├── cmd/
│   └── server/         # Main application entry point
├── config/             # Configuration management
├── handlers/           # HTTP request handlers
├── models/             # Data structures
├── services/           # Business logic
├── Makefile           # Common operations
├── Dockerfile         # Container configuration
├── go.mod             # Go module definition
└── README.md          # Project documentation
```

## Security

- All requests must include a valid API key in the Authorization header
- The webhook endpoint only accepts POST requests
- Environment variables are used for sensitive configuration
- Docker container runs with minimal privileges

## License

MIT 