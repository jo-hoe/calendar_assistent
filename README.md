# Calendar Assistent

[![Test Status](https://github.com/jo-hoe/calendar-assistent/actions/workflows/test.yml/badge.svg)](https://github.com/jo-hoe/calendar-assistent/actions?workflow=test)
[![Lint Status](https://github.com/jo-hoe/calendar-assistent/actions/workflows/lint.yml/badge.svg)](https://github.com/jo-hoe/calendar-assistent/actions?workflow=lint)
[![Go Report Card](https://goreportcard.com/badge/github.com/jo-hoe/calendar-assistent)](https://goreportcard.com/report/github.com/jo-hoe/calendar-assistent)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/calendar-assistent/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/calendar-assistent?branch=main)

An HTTP service that accepts artifacts (images, PDFs, text) and extracts calendar event information using an AI proxy, then creates events in Google Calendar.

## Overview

Calendar Assistent provides two HTTP API endpoints:

1. **POST /v1/events/artifact** — Upload a file (image, PDF, or text) containing event information
2. **POST /v1/events/text** — Submit plain text describing an event

Both endpoints use a pluggable LLM client to extract event details (title, time, location) and create a Google Calendar event.

## Quick Start

### Prerequisites

- Docker (or Go 1.24+ if running from source)
- Google Cloud Service Account with Calendar API access
- Optional: an OpenAI-compatible AI Proxy if using `llm.provider: "aiproxy"` (defaults to mock otherwise)

### Configure

1. Copy `config.example.yaml` to either:
   - `dev/app-config.yaml` (used by docker-compose), or
   - `config.yaml` in the project root (used for local runs)

2. Place your Google Service Account JSON credentials at `dev/google-credentials.json`

3. Minimum edits:
   - Set `calendar.google.credentialsFile` to the path of your credentials
   - Set `calendar.google.calendarId` (defaults to `"primary"`)
   - Choose LLM:
     - Mock (default): `llm.provider: "mock"` — works without external services
     - AI Proxy: set `llm.provider: "aiproxy"`, `llm.aiproxy.baseUrl`, and `llm.aiproxy.apiKey`

### Run

#### Using Docker Compose

```bash
docker compose up --build
```

#### From source

```bash
go run ./cmd/calendar-assistent
```

#### Call the API

Create event from text:

```bash
curl -X POST "http://localhost:8080/v1/events/text" \
  -H "Content-Type: application/json" \
  -d '{"text":"Team meeting tomorrow at 3pm in Room 42"}'
```

Create event from file:

```bash
curl -X POST "http://localhost:8080/v1/events/artifact" \
  -F "file=@ticket.png" \
  -H "X-API-Key: YOUR_API_KEY"  # only if apiKey is configured
```

Health check:

```bash
curl http://localhost:8080/healthz
```

## Local Development with k3d

```bash
# Start cluster + deploy
make start-k3d

# After code changes, redeploy
make upgrade-k3d

# Tear down
make delete-k3d
```

## Configuration

Create a `config.yaml` in the project root or set `CALENDAR_ASSISTENT_CONFIG` to the path of your config file. See `config.example.yaml` for a complete template.

### Supported file types

- `image/png`, `image/jpeg`, `image/gif`, `image/webp`
- `application/pdf`
- `text/plain`, `text/html`

## Security

- If `server.apiKey` is set, all API requests must include header `X-API-Key`
- Google credentials are mounted as K8s secrets in production

## License

See [LICENSE](LICENSE) file. This software may not be used for AI/ML training purposes.
