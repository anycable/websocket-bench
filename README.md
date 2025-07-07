# websocket-bench

CLI interface for benchmarking WebSocket servers

## Overview
websocket-bench allows you to perform load testing on WebSocket servers of various types, including standard WebSocket servers, ActionCable, Phoenix, Pusher, and AnyCable.

This project is inspired by the [websocket-shootout](https://github.com/hashrocket/websocket-shootout) project.

## Installation

### Prerequisites

- Go 1.16 or higher

### Building from source

```bash
git clone https://github.com/anycable/websocket-bench.git
cd websocket-bench

# Build the binary
go build -o websocket-bench
```

## Usage

websocket-bench provides three main benchmark commands:

### Echo Benchmark

Tests 1-to-1 performance by sending messages to the server and measuring how long it takes for the server to echo them back.

```bash
./websocket-bench echo [options] ws://server-url
```

Common options:
- `-c, --concurrent`: Number of concurrent echo requests (default: 50)
- `-s, --sample-size`: Number of echoes in a sample (default: 10000)
- `--step-size`: Number of clients to increase each step (default: 5000)
- `--server-type`: Server type to connect to (json, binary, actioncable, phoenix) (default: json)
- `-f, --format`: Output format (json or text)
- `-n, --filename`: Output filename

### Broadcast Benchmark

Tests 1-to-many performance by sending broadcast messages and measuring how long it takes for all clients to receive them.

```bash
./websocket-bench broadcast [options] ws://server-url
```

Common options:
- `-c, --concurrent`: Number of concurrent broadcast requests (default: 4)
- `--connect-concurrent`: Concurrent connection initialization requests (default: 100)
- `-s, --sample-size`: Number of broadcasts in a sample (default: 20)
- `--initial-clients`: Initial number of clients (default: 0)
- `--step-size`: Number of clients to increase each step (default: 5000)
- `--server-type`: Server type (`json`, `binary`, `actioncable`, `phoenix`, `pusher`, `anycable-pusher`) (default: `json`)
- `-f, --format`: Output format (`json` or `text`)
- `-n, --filename`: Output filename

**Pusher‑compatible flags** (for `pusher` and `anycable-pusher`):
- `--pusher-app-id`: Pusher App ID (default: `app-id`)
- `--pusher-app-key`: Pusher App Key (default: `app-key`)
- `--pusher-app-secret`: Pusher App Secret (default: `app-secret`)
- `--pusher-channel`: Channel name (default: `private-benchmark`)
- `--pusher-host`: HTTP trigger host (default: `127.0.0.1`)
- `--pusher-port`: HTTP trigger port (default: `6001`)

**Additional flags for `anycable-pusher` only**:
- `--anycable-backend`: Broadcast transport (`http` or `redis`) (default: `http`)
- `--anycable-http-endpoint`: AnyCable HTTP broadcast endpoint (default: `http://localhost:8080/_broadcast`)
- `--anycable-redis-addr`: Redis address for broadcasts (default: `127.0.0.1:6379`)

### Connect Benchmark

Tests connection initialization performance by measuring how long it takes to establish WebSocket connections.

```bash
./websocket-bench connect [options] ws://server-url
```

Common options:
- `-c, --concurrent`: Number of concurrent connection requests (default: 50)
- `--step-size`: Number of clients to connect at each step (default: 5000)
- `--server-type`: Server type to connect to (default: json)
  - `actioncable-connect`: Test connection to ActionCable servers
  - `pusher-connect`: Test connection to Pusher-compatible servers (with channel subscription)
  - `anycable-connect`: Test connection to AnyCable (similar to pusher-connect, but optimized for AnyCable)
- `-f, --format`: Output format (json or text)
- `-n, --filename`: Output filename
- `--pusher-channel`: Pusher channel name for connect tests (default Benchmark)

### Worker Mode

Worker mode allows you to distribute connections across multiple machines for scaling tests to a large number of connections.

```bash
./websocket-bench worker [options]
```

Options:
- `-a, --address`: Address to listen on (default: 0.0.0.0)
- `-p, --port`: Port to listen on (default: 3000)

When using worker mode, you can run the master process with the `--worker-addr` option to specify worker node addresses separated by commas.

### Example Output

```
[2025-06-24T21:00:17+09:00] clients:   500    95per-rtt:  25ms    min-rtt:   1ms    median-rtt:  12ms    max-rtt:  25ms
[2025-06-24T21:00:19+09:00] clients:  1000    95per-rtt: 109ms    min-rtt:   5ms    median-rtt:  25ms    max-rtt: 109ms
[2025-06-24T21:00:22+09:00] clients:  1500    95per-rtt:  63ms    min-rtt:   3ms    median-rtt:  44ms    max-rtt:  63ms
[2025-06-24T21:00:25+09:00] clients:  2000    95per-rtt:  94ms    min-rtt:  10ms    median-rtt:  53ms    max-rtt:  94ms
...
[2025-06-24T21:00:58+09:00] clients:  5000    95per-rtt: 474ms    min-rtt:  32ms    median-rtt: 149ms    max-rtt: 474ms
```

## Visualizing Results

You can generate JSON output files and visualize them using the included Ruby script:

```bash
# Run benchmark with JSON output
./websocket-bench echo -f json -n dist/echo_$(date +%s).json ws://server-url

# Generate HTML chart
ruby etc/chart.rb dist chart.html
```

This will generate an HTML file with charts showing the median, 95th percentile, and maximum RTT for each benchmark.

## Pusher and Soketi Testing
You can benchmark a Pusher Channels–compatible server [soketi](https://docs.soketi.app/) by running a local instance and pointing websocket‑bench at it

### 1. Run Soketi via Docker

```bash
docker run \
  --network host \
  -e SOKETI_DEBUG=1 \
  -p 6001:6001 \
  -p 9601:9601 \
  quay.io/soketi/soketi:1.4-16-debian
```

- WebSocket API will be listening on 127.0.0.1:6001
- HTTP API (for triggers) will also be on http://127.0.0.1:6001

It creates a single app with next accesses:
```
app_id=app-id
key=app-key
secret=app-secret
```

See the official [install guide](https://docs.soketi.app/getting-started/installation)

### Pusher connect url

Pushed client url looks like that:

```
ws://ws-[cluster].pusher.com:[port]/app/[APP_KEY]?client=[library]&version=[lib_version]&protocol=[protocol_version]
```

- scheme: ws or wss
- cluster: your cluster name (for soketi, this is omitted; use host directly)
- port: soketi defaults to 6001
- key: your Pusher App Key
- client / version / protocol: library metadata

**Soketi example:**
```
ws://127.0.0.1:6001/app/app-key?client=js&version=7.0.3&protocol=7
```

More details [here](https://pusher.com/docs/channels/library_auth_reference/pusher-websockets-protocol/)

### Running the Broadcast Pusher

```
./websocket-bench broadcast "ws://127.0.0.1:6001/app/app-key?client=js&version=7.0.3&protocol=7" \
  --server-type pusher \
  --concurrent 5 \
  --step-size 500 \
  --steps-delay 1 \
  --total-steps 10 
```

If your `app-id`, `app-key`, or `app-secret` differ from the defaults above, pass them via `--pusher-*` flags so that the HMAC signature is computed correctly for private/presence channels


## Testing AnyCable

websocket-bench supports multiple modes for working with AnyCable:

- `anycable-pusher`: For testing broadcasting messages using the Pusher-compatible protocol
- `anycable-connect`: For testing connection performance to AnyCable servers

### HTTP broadcast

```bash
    ./websocket-bench broadcast \
    "ws://localhost:8080/app/app-key?client=bench&version=1.0&protocol=7" \
    --server-type anycable-pusher \
    --pusher-app-id app-id \
    --pusher-app-key app-key \
    --pusher-app-secret app-secret \
    --anycable-backend http \
    --concurrent 5 --step-size 500 --steps-delay 1 --total-steps 10
```

### Redis backend

```bash
    ./websocket-bench broadcast \
    "ws://localhost:8080/app/app-key?client=bench&version=1.0&protocol=7" \
    --server-type anycable-pusher \
    --pusher-app-id app-id \
    --pusher-app-key app-key \
    --pusher-app-secret app-secret \
    --anycable-backend redis \
    --anycable-redis-addr 127.0.0.1:6379 \
    --concurrent 5 --step-size 500 --steps-delay 1 --total-steps 10
```

### Connection performance testing

```bash
    ./websocket-bench connect \
    "ws://localhost:8080/app/app-key?client=bench&version=1.0&protocol=7" \
    --server-type anycable-connect \
    --pusher-app-id app-id \
    --pusher-app-key app-key \
    --pusher-app-secret app-secret \
    --concurrent 50 --step-size 500 --steps-delay 1 --total-steps 10
```

> Note: The `anycable-connect` server type works similarly to `pusher-connect` but is corrected for AnyCable - it skips the channel unsubscribe phase during connection closing, which is not needed for AnyCable
