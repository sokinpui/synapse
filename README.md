# Synapse

Simple distributed task queue system implemented with HTTP and Go channels.

## Architecture

The system consists of two main components: an HTTP `server` (providing an OpenAI-compatible API) and in-process `worker` pools communicating through an in-memory channel broker.

```
Client ---HTTP---> Server <---Go Channels---> Worker
```

## How it Works

You can configure multiple API keys for upstream providers. Synapse uses key balancing to distribute requests across those keys, automatically retrying on failure across available keys. This scales throughput linearly with the number of API keys without manual management.

## Prerequisites

- Go (1.24+)

## Getting Started

### 1. Configuration

The server and provider registry are configured in `config.go`. API keys are loaded from environment variables (or a `.env` file).

Set up your environment in `.env` or export them directly:

```sh
# Comma-separated or newline-separated API keys
AISRP_API_KEYS="key1,key2,key3"
```

To configure network proxies (optional):

```env
http_proxy=http://127.0.0.1:1087
https_proxy=http://127.0.0.1:1087
ALL_PROXY=socks5://127.0.0.1:1080
```

### 2. Run

Start the server locally using the startup script:

```sh
chmod +x start.sh
./start.sh
```

### 3. Docker

Run the service using Docker Compose:

```sh
docker compose up -d
```

The server listens on HTTP port `9001` by default.

## OpenAI Compatible API

Synapse serves OpenAI-compatible endpoints. Models are exposed using `<provider>/<model_code>` naming convention (for example, `aisrp/gemini-flash-latest`), and Synapse strips the provider prefix when routing requests to the target provider.

### List Models

```sh
curl http://localhost:9001/v1/models
```

### Chat Completions

**Streaming:**

```sh
curl http://localhost:9001/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aisrp/gemini-flash-latest",
    "messages": [{"role": "user", "content": "Say hello!"}],
    "stream": true
  }'
```

**Non-Streaming:**

```sh
curl http://localhost:9001/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aisrp/gemini-flash-latest",
    "messages": [{"role": "user", "content": "Say hello!"}],
    "stream": false
  }'
```

### Image Generations

```sh
curl http://localhost:9001/v1/images/generations \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aisrp/gemini-3.1-flash-lite-image",
    "prompt": "A sunset over mountains"
  }'
```
