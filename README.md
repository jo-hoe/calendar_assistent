# Calendar Assistent

[![Test Status](https://github.com/jo-hoe/calendar_assistent/actions/workflows/test.yml/badge.svg)](https://github.com/jo-hoe/calendar_assistent/actions?workflow=test)
[![Lint Status](https://github.com/jo-hoe/calendar_assistent/actions/workflows/lint.yml/badge.svg)](https://github.com/jo-hoe/calendar_assistent/actions?workflow=lint)
[![Go Report Card](https://goreportcard.com/badge/github.com/jo-hoe/calendar_assistent)](https://goreportcard.com/report/github.com/jo-hoe/calendar_assistent)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/calendar_assistent/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/calendar_assistent?branch=main)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jo-hoe/calendar_assistent)](https://go.dev/)
[![Go Reference](https://pkg.go.dev/badge/github.com/jo-hoe/calendar_assistent.svg)](https://pkg.go.dev/github.com/jo-hoe/calendar_assistent)
[![Release](https://img.shields.io/github/v/release/jo-hoe/calendar_assistent)](https://github.com/jo-hoe/calendar_assistent/releases)
[![Docker Image](https://img.shields.io/badge/ghcr.io-jo--hoe%2Fcalendar__assistent-blue?logo=docker)](https://github.com/jo-hoe/calendar_assistent/pkgs/container/calendar_assistent)
[![Helm Chart](https://img.shields.io/badge/helm-calendar--assistent-blue?logo=helm)](https://jo-hoe.github.io/calendar_assistent)

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

## Google Calendar Setup

To let the service write events into a Google Calendar, you need a Google Cloud **service account** and a dedicated calendar that the service account is allowed to edit. The steps below produce a credentials JSON file that the app reads via `calendar.google.credentialsFile`.

### 1. Create a Google Cloud project

1. Open the [Google Cloud Console](https://console.cloud.google.com/).
2. Create a new project (or pick an existing one) — note its **project ID**.

### 2. Enable the Google Calendar API

1. In the Cloud Console, go to **APIs & Services → Library**.
2. Search for **Google Calendar API** and click **Enable**.

### 3. Create the service account

1. Go to **IAM & Admin → Service Accounts → Create service account**.
2. Pick a name (e.g. `calendar-assistent`); the email will look like `calendar-assistent@<project-id>.iam.gserviceaccount.com`. Copy this address — you'll need it in step 5.
3. Skip the optional "Grant this service account access to project" and "Grant users access" steps; calendar access is granted per-calendar instead.

### 4. Create and download a JSON key

1. Open the service account, go to the **Keys** tab → **Add key → Create new key → JSON**.
2. Save the downloaded file. Place it at the path referenced by `calendar.google.credentialsFile` (for local dev: `dev/google-credentials.json`).
3. Treat this file like a password — never commit it. The repo's `.gitignore` already excludes `dev/google-credentials.json`.

### 5. Create a dedicated calendar and share it with the service account

A service account has its own (mostly unusable) primary calendar, so you'll want a separate calendar that you can also view in your normal Google account.

1. In [Google Calendar](https://calendar.google.com/), open the left sidebar → **Other calendars → + → Create new calendar**.
2. Give it a name (e.g. `Assistent`) and create it.
3. After creation, find it under **My calendars**, hover it, click ⋮ → **Settings and sharing**.
4. Under **Share with specific people or groups**, click **Add people** and paste the service account email from step 3.
5. Set permission to **Make changes to events** (writing) and save.
6. Scroll down to **Integrate calendar** and copy the **Calendar ID** (looks like `abc123…@group.calendar.google.com`).

### 6. Wire it into the config

```yaml
calendar:
  provider: "google"
  google:
    credentialsFile: "/app/secrets/google/google-credentials.json"   # path inside the container
    calendarId: "abc123…@group.calendar.google.com"                   # from step 5
    timeZone: "Europe/Berlin"
```

For Kubernetes deployments, supply the JSON content as `googleCredentials` in your Helm values — the chart mounts it at `/app/secrets/google/google-credentials.json`.

### Notes

- The service account does **not** need domain-wide delegation for this flow — direct calendar sharing is sufficient.
- If the app logs `403 forbidden` when creating events, the service account is missing the **Make changes to events** permission on the target calendar.
- To remove access later, delete the service account from the calendar's sharing list and/or rotate the JSON key from the **Keys** tab.

## Webcal / ICS Provider (S3)

Instead of writing directly to Google Calendar, the `webcal` provider maintains a single `.ics` file on S3. Any iCal-compatible client (Google Calendar, Apple Calendar, Outlook) can subscribe to the file URL and display the events.

### How it works

On every `CreateEvent` call the app:
1. Downloads the current `.ics` from S3 (creates a new file if none exists yet).
2. Removes events whose end time is older than `eventTtl` (default 30 days).
3. Appends the new VEVENT.
4. Uploads the result back to S3.

### S3 credentials file

Create a JSON file with your AWS credentials and mount it as a Kubernetes Secret:

```json
{"accessKeyId":"AKIA…","secretAccessKey":"…"}
```

Supply the file content as `s3Credentials` in your Helm values — the chart mounts it at `/app/secrets/s3/s3-credentials.json`.

For local dev, place the file somewhere accessible and point `calendar.webcal.storage.s3.credentialsFile` at it.

The `endpoint` field is optional and lets you point at a MinIO or LocalStack instance instead of real AWS.

### Config

```yaml
calendar:
  provider: "webcal"
  webcal:
    eventTtl: "720h"   # 30 days
    storage:
      provider: "s3"
      s3:
        bucket: "my-bucket"
        key: "calendar.ics"
        region: "eu-central-1"
        credentialsFile: "/app/secrets/s3/s3-credentials.json"
        endpoint: ""       # optional: MinIO / LocalStack URL
        publicUrl: "https://my-bucket.s3.eu-central-1.amazonaws.com/calendar.ics"
```

### Subscribing in Google Calendar

1. Open [Google Calendar](https://calendar.google.com/) → **Other calendars → From URL**.
2. Paste the S3 public URL of the `.ics` file (e.g. `https://my-bucket.s3.eu-central-1.amazonaws.com/calendar.ics`).
3. Click **Add calendar**.

> **Note:** Google refreshes external calendars infrequently (up to 24 hours). New events may not appear immediately.

## SMTP Calendar Provider

The `smtp` provider sends a `METHOD:REQUEST` iCalendar email to the configured recipient. Most email clients (Outlook, Apple Mail, Thunderbird, Gmail) recognise this format and present the recipient with **Accept / Decline / Tentative** buttons, adding the event to their calendar on acceptance.

### How it works

On every `CreateEvent` call the app:
1. Builds a VEVENT wrapped in a `text/calendar; method=REQUEST` MIME part.
2. Connects to the configured SMTP server (STARTTLS on port 587 by default, implicit TLS on port 465 when `tls: true`).
3. Authenticates using the chosen `authMethod` (`plain`, `login`, or `none` for open relays).
4. Sends the message from `from` to `to`.

### Credentials file

Create a JSON file with your SMTP credentials:

```json
{"username":"smtp-user@example.com","password":"app-password","organizer":"mailto:sender@example.com"}
```

- `username` / `password` — SMTP login credentials.
- `organizer` — shown as the meeting organiser in the recipient's calendar app (use `mailto:` prefix).
- For `authMethod: "none"` (open relay) the credentials file is not required.

### Config

```yaml
calendar:
  provider: "smtp"
  smtp:
    host: "smtp.gmail.com"
    port: 587
    authMethod: "plain"   # "none", "plain", or "login"
    credentialsFile: "/app/secrets/smtp/smtp-credentials.json"
    from: "Calendar Assistent <noreply@example.com>"
    to: "recipient@example.com"
    tls: false            # set true for port 465 (implicit TLS); false uses STARTTLS on port 587
```

### Kubernetes / Helm

Supply the credentials file content (base64-encoded) as `smtpCredentials` in your Helm values:

```yaml
smtpCredentials: "<base64-encoded JSON>"
```

The chart creates a Kubernetes Secret and mounts it at `/app/secrets/smtp/smtp-credentials.json`, which matches the default `credentialsFile` path above.

### Notes

- Gmail requires an **App Password** (not your regular account password). Generate one at [myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords).
- Use port **465** with `tls: true` for implicit TLS; use port **587** with `tls: false` for STARTTLS (recommended default).
- For open relay / internal SMTP servers, set `authMethod: "none"` and omit the credentials file.

### Provider comparison

| Provider | Writes to | Recipient action |
|----------|-----------|-----------------|
| google   | Google Calendar directly | None — event appears immediately |
| webcal   | S3 `.ics` file | Subscribe once; updates appear with delay |
| smtp     | Email inbox | Accept / Decline prompt |

## Security

- If `server.apiKey` is set, all API requests must include header `X-API-Key`
- Google credentials are mounted as K8s secrets in production

## License

See [LICENSE](LICENSE) file. This software may not be used for AI/ML training purposes.
