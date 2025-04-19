# Alert Manager Telegram Message Proxy Webhook

A production-ready webhook service that forwards messages to Telegram. This service acts as a proxy between your applications and Telegram, allowing you to send messages to specific chat IDs through a secure webhook. It also supports AlertManager webhook integration for sending Prometheus alerts to Telegram.

## Features

- Secure webhook endpoint with API key authentication
- Support for text messages
- Support for media attachments (photos, videos, documents)
- AlertManager webhook integration
- Environment variable configuration
- Docker containerization
- Production-ready structure

## Prerequisites

- Go 1.21 or later
- Docker (optional, for containerization)
- A Telegram bot token (get it from [@BotFather](https://t.me/BotFather))
- Telegram chat ID (for receiving alerts)

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
   LOG_LEVEL=info
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

## Docker Compose

The project includes a `docker-compose.yml` file for easy deployment:

1. Build and start the container:
   ```bash
   docker-compose up -d
   ```

2. View logs:
   ```bash
   docker-compose logs -f
   ```

3. Stop the container:
   ```bash
   docker-compose down
   ```

## API Usage

### General Webhook

Send a POST request to `/webhook` with the following JSON body:

```json
{
  "chat_id": 123456789,
  "message": "Hello, World!",
  "media_url": "https://example.com/image.jpg",
  "media_type": "photo"
}
```

#### Headers
- `Authorization: Bearer your_api_key`

#### Media Types
Supported media types:
- `photo`
- `video`
- `document`

### AlertManager Webhook

Send alerts to `/alertmanager/webhook` endpoint. The service expects AlertManager's webhook format.

#### AlertManager Configuration Example

```yaml
global:
  resolve_timeout: 15s

route:
  group_by: ['alertname']
  group_wait: 1s
  group_interval: 1s
  repeat_interval: 1m
  receiver: 'telegram'

receivers:
- name: 'telegram'
  webhook_configs:
  - url: 'http://your-server:8080/alertmanager/webhook'
    send_resolved: true
    http_config:
      authorization:
        type: Bearer
        credentials: 'your_api_key'
```

#### Adding Telegram Chat ID to Prometheus Alerts

You can add the `chat_id` label to your Prometheus alert rules. Here's an example:

```yaml
groups:
- name: example
  rules:
  - alert: HighCPUUsage
    expr: cpu_usage > 80
    for: 5m
    labels:
      severity: warning
      chat_id: "-12345678901011"  # Your Telegram chat ID
    annotations:
      summary: "High CPU usage detected"
      description: "CPU usage is above 80% for 5 minutes"
```

The `chat_id` label can be added to:
1. Individual alerts in your Prometheus rules
2. Alert groups using the `labels` field
3. Alert templates if you're using them

#### Getting Telegram Chat ID
1. Add your bot to a group/channel
2. Send a message in the group/channel
3. Forward that message to @userinfobot on Telegram
4. The bot will reply with the chat ID

## Project Structure

```
.
├── cmd/
│   └── server/         # Main application entry point
├── config/             # Configuration management
├── handlers/           # HTTP request handlers
│   ├── webhook.go      # General webhook handler
│   └── alertmanager.go # AlertManager webhook handler
├── models/             # Data structures
│   ├── webhook.go      # General webhook models
│   ├── queue.go        # Queue models
│   └── alertmanager.go # AlertManager models
├── pkg/
│   └── logger/         # Logging utilities
├── services/           # Business logic
│   ├── telegram.go     # Telegram service
│   └── queue.go        # Message queue service
├── Makefile           # Common operations
├── Dockerfile         # Container configuration
├── docker-compose.yml # Docker compose configuration
├── go.mod             # Go module definition
└── README.md          # Project documentation
```

## Security

- All requests must include a valid API key in the Authorization header
- The webhook endpoints only accept POST requests
- Environment variables are used for sensitive configuration
- Docker container runs with minimal privileges

## License

MIT 