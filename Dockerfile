# Build Stage
FROM golang:1.24-bookworm AS builder

WORKDIR /src

RUN apt-get update && apt-get install -y ca-certificates

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /bin/synapse-server .

# Runtime Stage
FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/synapse-server .

EXPOSE 9001

ENTRYPOINT ["./synapse-server"]
