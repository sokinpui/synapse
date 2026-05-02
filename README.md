# Synapse

Simple distributed task queue system implement in HTTP and Go Channels

## Architecture

The system consists of two main components: a `server` (supporting REST/JSON and OpenAI compatibility) and a `worker` running within the same process.

```
Client ---HTTP---> Server <---Go Channels---> Worker
```

## Prerequisites

- Go (1.24+)

## Getting Started

### 1. Configuration

The application is configured using a `config.yaml` file. API keys for the LLM providers are configured using environment variables.

Create a `config.yaml` file in the root directory with the following content:

```yaml
server:
  http_port: 8080

worker:
  concurrency_multiplier: 4

models:
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai"
    codes:
      - "gemini-3-flash-preview"
      - "gemini-3.1-flash-lite-preview"

  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    codes:
      - "z-ai/glm-4.5-air:free"
      - "qwen/qwen3-coder:free"
```

# API key for the underlying LLM provider

Then, export the necessary API keys:

```sh
export GENAI_API_KEYS="YOUR_GEMINI_API_KEY_1,YOUR_GEMINI_API_KEY_2"
export OPENROUTER_API_KEY="YOUR_OPENROUTER_API_KEY"
```

### 2. Run

Tidy modules and build the server binary:

```sh
./start.sh
```

### 3. Docker

```
docker compose up -d
```

The server will listen for HTTP requests on the port specified in `config.yaml`.

## OpenAI Compatible API

You can use any OpenAI-compatible client by pointing it to the Synapse server.

**List Models:**

```
curl http://localhost:8080/v1/models
```

**Chat Completions:**

```
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [{"role": "user", "content": "Say hello!"}],
    "stream": true
  }'
```
