# interline-engine

[![CI](https://github.com/hellprise/interline-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/hellprise/interline-engine/actions/workflows/ci.yml)

Go search and booking engine for the Interline flight aggregator.

## Getting Started

### Local database

Install `goose` CLI (once):

```bash
go install github.com/pressly/goose/v3/cmd/goose@v3.28.0
```

From scratch:

```bash
cp .env.example .env # set GOOSE_DB_PASSWORD in .env
docker compose up -d --wait
goose up
```

### Build

```bash
go build ./...
```

### Test

Run unit tests with race detection enabled:

```bash
go test -race ./...
```
