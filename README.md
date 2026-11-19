# Alertmanager Telegram Webhook Relay

A Go service that turns application webhooks and Prometheus Alertmanager notifications into Telegram bot messages. It does not scrape Prometheus, evaluate alert rules, receive Telegram updates, or store message history.

```text
Application POST /webhook ──────────────┐
Alertmanager POST /alertmanager/webhook ├─> auth + validation ─> in-memory queue ─> Telegram Bot API
                                       ┘
GET /health ─────────────────────────────> process liveness (does not contact Telegram)
```

## Scope and acceptance criteria

| ID | Contract |
| --- | --- |
| WEB-1 | Both webhooks require a Bearer key. Accept one bounded JSON object per request, validate fields before queueing, and return `202`, not a delivery receipt. |
| ALERT-1 | Route each firing/resolved alert using its own `chat_id` or the common label. Admit the whole batch or none. |
| ALERT-2 | Render a readable digest per destination chat. Preserve alert order and status within that request, number pages when needed, and keep each page within the Telegram text limit. Never mix chats. |
| ALERT-2A | Single alerts omit the digest counts and entry number. Show summary and multiline details as plain text, omit duplicate summary/details and the raw Prometheus `generatorURL`; batching, routing, and `/webhook` are unchanged. |
| ALERT-2B | Show a clickable `Open in Prometheus` label for each valid alert source, without displaying the raw URL or adding link previews. Keep the original query and correct link-to-alert mapping after pagination. Skip missing or invalid sources without rejecting the alert. |
| ALERT-3 | Send a chat's pages sequentially within a request. Retry only the current page. Count all pending pages toward queue capacity and reject the entire request if they do not fit. |
| SEND-1 | Send text and optional URL media. Retry temporary failures with timeouts and rate limits. A media retry does not replay successful text. Never log tokens, message text, or media URLs. |
| OPS-1 | Validate configuration at startup, expose process liveness, and stop taking HTTP traffic before draining the queue on termination. |

Batching applies within one Alertmanager webhook, not across requests. The general-purpose `/webhook` payload and HTTP responses stay unchanged. A durable queue and cross-request ordering are out of scope.

## Run locally

You need a token from [BotFather](https://t.me/BotFather), a numeric Telegram chat ID, and a bot that can post to that chat. For direct messages, start the conversation with the bot first. Generate a strong, unique webhook key (for example, `openssl rand -hex 32`). Copy `.env.example` to `.env` and replace both credential placeholders. Keep `.env` private.

**Docker Compose** (recommended):

```sh
docker compose config --quiet
docker compose up -d --build
curl --fail --silent --show-error http://127.0.0.1:8080/health
docker compose logs --tail=50 telegram-webhook
```

Compose automatically reads `.env` and publishes to loopback by default. For a remote sender, put an HTTPS reverse proxy in front of the service or place both services on a trusted private network with TLS. If an external proxy cannot reach loopback, configure `PUBLISH_ADDRESS` for a trusted host interface and restrict it with firewall rules. Do not expose the plaintext upstream directly to the Internet.

**Native Go** (Go 1.21 minimum; use a supported release for deployments):

```sh
set -a
. ./.env  # Source only a trusted shell-compatible file you created.
set +a
make run
```

Go does not read `.env` directly. The native listener binds on all interfaces, so restrict access in your environment. `make build` writes `bin/server`. Use the built binary for supervision and testing OS signals.

| Variable | Meaning |
| --- | --- |
| `TELEGRAM_BOT_TOKEN` | Required Telegram bot token. Startup checks the format, not live permissions or token validity. |
| `WEBHOOK_API_KEY` | Required case-sensitive shared key without spaces. A key holder can send to any chat reachable by this bot; use a dedicated bot and share the key only with trusted senders. |
| `SERVER_PORT` | Defaults to `8080`. Native listen port or Compose's published host port; the Compose container always listens on `8080`. |
| `LOG_LEVEL` | `debug`, `info` (default), `warn`, or `error`. |
| `PUBLISH_ADDRESS` | Compose-only bind address on the host (default `127.0.0.1`). |

## HTTP contract

| Method and path | Result |
| --- | --- |
| `POST /webhook` | Queue a text message, media, or both. |
| `POST /alertmanager/webhook` | Queue a paginated digest for each destination chat in the Alertmanager batch. |
| `GET` or `HEAD /health` | Unauthenticated liveness: `200`; GET returns `{"status":"healthy"}`. This is not a check of queue capacity, Telegram credentials, or delivery. |

Both POST routes accept JSON up to 1 MiB and require `Authorization: Bearer <WEBHOOK_API_KEY>`. A successful response is `202` with `{"status":"queued"}`. Errors: `400` invalid JSON or payload, `401` invalid or missing credentials, `405` wrong method, `413` oversized body, or `503` if the queue cannot admit the entire request. On `503`, back off and retry. Avoid automatically retrying successful `202` responses.

### Application webhook

```json
{
  "chat_id": -123456789,
  "message": "Deployment finished",
  "media_url": "https://example.com/report.pdf",
  "media_type": "document"
}
```

Use a nonzero numeric `chat_id` and either nonblank `message` text or both media fields. Text is plain, up to 4,096 Unicode characters. `media_type` can be `photo`, `video`, or `document`. `media_url` must be an absolute HTTP(S) URL (maximum 4,096 bytes, without embedded credentials). Telegram fetches the media: it must be reachable by Telegram and satisfy the Bot API limits for the requested type. Do not pass confidential URLs. With both text and media, the relay attempts text first and media separately.

### Prometheus Alertmanager

Add a numeric `chat_id` label as a string to each Prometheus alert rule intended for the relay, for example `chat_id: "-123456789"`. The relay prefers the individual alert label and falls back to `commonLabels.chat_id` when the alert has no such label. If any alert is invalid, the service rejects the whole batch. Each alert must have a `firing` or `resolved` status.

```text
📟 Alertmanager

🔴 FIRING · HighCPU
Severity: CRITICAL · Instance: worker-1
Since: 2026-11-19 08:27 UTC

CPU usage crossed 90%.

Details
1. Check the top CPU consumers.
2. Compare usage with the last deployment.

🔗 Open in Prometheus
```

Single alerts omit counts and numbering. Multi-alert digests show firing and resolved counts and up to 10 alerts per message, paginated as `Page 1/N`, `Page 2/N`, etc. Each chat only sees its own alerts. The 4,096-character message budget is conservative for emoji; long display fields are shortened with `…`. Each alert with a valid `generatorURL` gets a tappable **Open in Prometheus** label that opens the original query without printing the URL. Missing, invalid, or overly long URLs are skipped. Runbook links in annotations are kept, text is not interpreted as HTML or Markdown, and Telegram link previews are disabled.

If the link opens a cluster-only hostname, set Prometheus [`--web.external-url`](https://prometheus.io/docs/prometheus/latest/command-line/prometheus/) to the address that responders can reach (for example, the existing authenticated Prometheus ingress). The relay preserves the sender's URL; it does not rewrite it or expose Prometheus.

```yaml
route:
  group_by: ['alertname', 'chat_id']
  receiver: telegram-relay

receivers:
  - name: telegram-relay
    webhook_configs:
      - url: 'https://relay.example.com/alertmanager/webhook'
        send_resolved: true
        max_alerts: 500
        http_config:
          authorization:
            type: Bearer
            credentials_file: '/etc/alertmanager/secrets/telegram-relay-key'
```

The relay accepts up to 500 alerts per webhook and a 1 MiB request body. `max_alerts` truncates larger groups at the sender and the relay logs the reported omitted count. Use sufficiently selective grouping to avoid dropped alerts. Keeping `alertname` in `group_by` gives one digest per alert type and chat; grouping by `chat_id` alone can aggregate more alert types but may need shorter group intervals or an additional meaningful label. Separate webhook requests are not merged.

## Reliability, deployment, and rollback

- The queue holds up to 5,000 pending jobs, including in-flight jobs and every digest page. Pages for one chat from one request are sent sequentially, with other queued work able to run between them. A failed attempt retries only its own page. Each instance starts about 30 Telegram API calls per minute under load; text, media, and retries each use a slot. A large backlog can take hours to clear. `429` pauses new calls from that instance according to Telegram's retry delay. Transport errors and server errors are retried, up to three attempts per operation; permanent errors are logged without retry.
- On `SIGTERM` or Ctrl-C, the service allows up to 10 seconds for HTTP to finish, then up to 30 seconds for queue work. Compose gives it 45 seconds before a forced stop. The process logs and exits nonzero if accepted jobs have not finished at the deadline. Terminal Telegram delivery failures are logged separately. Alert on repeat delivery failures and test the bot against a dedicated staging chat.
- **No delivery or uptime guarantee:** `202` means accepted in RAM. Crashes or forced restarts can discard jobs, retries can duplicate a delivery when Telegram's response is lost, and pages from separate requests can interleave. Zero-downtime releases with crash-safe delivery require a durable shared queue, deduplication, and a staged multi-instance rollout; those are outside this service's current scope.
- Before upgrading, retain the previous image and its Compose configuration. The new Compose default bind is loopback; plan connectivity and TLS first if callers formerly connected directly. Validate ingress, liveness, and a firing/resolved notification in staging. Roll back by draining the new instance and starting the previous image/configuration; no database migration is required, and a rollback cannot recover lost in-memory jobs.

### Docker on an SSH-accessible VM

1. Verify Docker Engine and the Compose plugin are installed with `docker version` and `docker compose version`. Review the existing listeners and firewall before configuring ingress.
2. On the VM, place a rotated bot token and new webhook key in the project `.env` with mode `0600`. Do not paste secrets into chat, pass them as CLI arguments, or include `.env` in the uploaded build context. Run `docker compose config --quiet`, then `docker compose up -d --build --wait`.
3. Keep the default loopback bind. For operator-only access, use `ssh -N -L 18080:127.0.0.1:8080 vm-ssh-alias`, then open `http://127.0.0.1:18080/health` locally. For network webhook clients, use a TLS-terminating reverse proxy and restrict access to the intended senders. For `https://<ip>:8080`, the certificate must cover that IP and be trusted by Alertmanager; hostname-only certificates do not work on a raw IP URL. The SSH tunnel does not make the service reachable by Alertmanager unless Alertmanager can use that tunnel.
4. Check `/health` from the intended ingress, verify unauthorized requests return `401`, then send a grouped firing/resolved test notification to a dedicated Telegram chat. Check page order and confirm the bot is not posting to a production chat during staging.

## Verification

```sh
make test
go test -race ./...
go vet ./...
golangci-lint run ./...
make build
```

The tests exercise request admission and Alertmanager routing, worker shutdown and retry limits, and Telegram HTTP requests against a local fake API. The repository has no third-party Go dependencies. See `cmd/server` for startup and shutdown, `config` for environment validation, `handlers` and `models` for the HTTP contract, and `services` for queue and Telegram delivery.

**Verification record (2026-11-19):** `go test -race -coverpkg=./... -coverprofile=coverage.out ./...` passed with 88.8% statement coverage. Tests covered 500-alert pagination, mixed recipients, emoji sizing, fair scheduling, page retry and queue draining. The formatter check, `go vet ./...`, `golangci-lint run ./...`, `make build`, and a Linux amd64 cross-build passed. Docker Compose `config --quiet`, `build --quiet`, and `up -d --wait` passed in an isolated project on the local `desktop-linux` context with dummy credentials. Published loopback health and authentication rejection, read-only filesystem, dropped capabilities, and clean `SIGTERM` exit passed. No production deployment or live Telegram delivery was tested. Before rollout, verify a grouped firing/resolved notification through the intended HTTPS ingress into a dedicated Telegram chat.

**VM deployment (2026-11-19):** Installed Ubuntu's Docker Engine and Compose packages. Loaded a checksum-verified Linux amd64 image. The initial loopback deployment passed webhook authentication and validation checks, and Telegram `getMe` succeeded from the running container. Docker Compose then published port `8080` on the public interface; external HTTP checks returned `200` for health and `401` for the unauthenticated Alertmanager route. No Telegram test message was sent and HTTPS is not configured.

**Alert layout (ALERT-2A):** The new formatting passed `go test -race ./...`, `go vet ./...`, `golangci-lint run ./...`, the formatter check, and a local Docker image build that runs `go test ./...`. Regression tests cover source URL omission, retained annotation links and spacing-sensitive runbook text, deduplication, and 500-alert pagination. Use those tests for the formatting check and verify health and webhook authentication after each rollout.

**Prometheus source links (ALERT-2B):** Race tests, `go vet ./...`, `golangci-lint run ./...`, the formatter check, and the local Docker build passed. Tests verify UTF-16 link spans across pages, Telegram API payloads, and omission of invalid source links. No live Telegram message was sent and this change was not deployed.

## License

MIT
