# Synapse

Simple distributed task queue system implement in HTTP and Go Channels

## Architecture

The system consists of two main components: a `server` (supporting REST/JSON and OpenAI compatibility) and a `worker` running within the same process.

```
Client ---HTTP---> Server <---Go Channels---> Worker
```

## How it Works

You can configure multiple API keys for the same provider, and Synapse will use simple round-robin balancing to distribute requests across those keys. This allows you to scale your throughput linearly with the number of API keys you have, without mannually managing which key to use for each request.

## Prerequisites

- Go (1.24.1+)

## Getting Started

### 1. Configuration

The application is configured using a `config.yaml` file. API keys for the LLM providers are configured using environment variables specified in the config.

Create a `config.yaml` file in the root directory with the following content:

```yaml
server:
  http_port: 9001

worker:
  concurrency_multiplier: 4

models:
  # Example for Gemini Models
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai"
    env: GENAI_API_KEYS
    codes:
      - "gemini-2.5-flash"
      - "gemini-2.0-flash"
      - "gemini-3.1-pro-preview"

  # Example for OpenRouter Models
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    env: OPENROUTER_API_KEY
    codes:
      - "z-ai/glm-4.5-air:free"
      - "qwen/qwen3-coder:free"

  # How to add a new provider:
  ProviderC:
    # Openai api compatible provider that support /chat/completions
    base_url: "https://api.provider-c.com/v1"
    env: PROVIDER_C_API_KEY
    codes:
      - "model-a"
      - "model-b"
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
curl http://localhost:9001/v1/models
```

**Chat Completions:**

```
curl http://localhost:9001/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [{"role": "user", "content": "Say hello!"}],
    "stream": true
  }'
```
