FROM golang:1.24-alpine3.21 AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/calendar-assistent ./cmd/calendar-assistent

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tini

RUN mkdir -p /app/config /app/secrets

COPY --from=builder /out/calendar-assistent /app/calendar-assistent

ENV CALENDAR_ASSISTENT_CONFIG=/app/config/config.yaml

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/calendar-assistent"]
