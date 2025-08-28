![logo](relay.logo.large.png)

# Relay

A high-performance TCP relay service that receives Zscaler ZPA LSS (Log Streaming Service) data and forwards it to Splunk HEC (HTTP Event Collector). The application acts as an efficient middleware layer that batches log events for optimized delivery to Splunk.

## Features

- **TCP Server**: Accepts incoming connections from Zscaler ZPA LSS
- **Event Batching**: Configurable batch size and timeout for efficient Splunk delivery
- **JSON Parsing**: Intelligent parsing with fallback to raw text for malformed data
- **Splunk HEC Integration**: Direct integration with Splunk's HTTP Event Collector
- **Configuration Flexibility**: YAML configuration file or command-line flags
- **Template Generation**: Built-in configuration template generator
- **Graceful Shutdown**: Handles system signals for clean service termination

## How it Works

```mermaid
sequenceDiagram
    autonumber
    participant AC as ZPA App Connector (appliance)
    participant ZC as Zscaler Cloud
    participant LSS as ZPA Log Streaming Service
    participant R as Relay
    participant FS as Local Storage (NDJSON)
    participant HEC as Splunk HEC (/services/collector/raw)

    Note over AC,ZC: Operational & audit data flows from App Connector up to Zscaler Cloud
    AC-->>ZC: Telemetry, audit, user/app metrics (proprietary)

    Note over LSS,R: LSS initiates outbound connection to your receiver
    LSS->R: TCP/TLS connect to relay:port
    activate R

    loop Streaming (long-lived socket)
      LSS-->>R: NDJSON event "\n" (one JSON object per line)
      R->>FS: append line to zpa-YYYY-MM-DD.ndjson
      alt HEC forwarding enabled
        R->>HEC: HTTPS POST (optional gzip) with original JSON line
        HEC-->>R: 2xx (ingested)  / non-2xx (retry/backoff)
      end
    end
    deactivate R

    Note over R,HEC: Relay may batch/retry; Splunk indexes by event time or ingest time depending on props
```

## Requirements

- Go 1.24.4 or later
- Access to a Splunk instance with HEC enabled
- Network connectivity between Zscaler ZPA and the relay service

## Installation

### From Source

```bash
git clone https://github.com/scottbrown/relay.git
cd relay
go build -o relay main.go
```

### Using Task Runner

If you have [Task](https://taskfile.dev/) installed:

```bash
task build
```

This creates the binary at `.build/relay`.

## Configuration

### Configuration File

Create a YAML configuration file (default location: `/etc/relay/config.yml`):

```yaml
listen_port: "9514"
splunk_hec_url: "https://your-instance.splunkcloud.com:8088/services/collector"
splunk_token: "your-hec-token-here"
source_type: "zscaler:zpa:lss"
index: "zscaler"
batch_size: 100
batch_timeout: "5s"
```

### Generate Configuration Template

```bash
./relay -t > config.yml
```

### Configuration Options

| Option | Description | Required | Default |
|--------|-------------|----------|---------|
| `listen_port` | TCP port to listen on | No | `9514` |
| `splunk_hec_url` | Splunk HEC endpoint URL | Yes | - |
| `splunk_token` | Splunk HEC authentication token | Yes | - |
| `source_type` | Splunk sourcetype for events | No | `zscaler:zpa:lss` |
| `index` | Splunk index name | No | `zscaler` |
| `batch_size` | Number of events per batch | No | `100` |
| `batch_timeout` | Maximum time to wait before sending batch | No | `5s` |

## Usage

### Basic Usage

```bash
./relay -f /path/to/config.yml
```

### Command-Line Options

```bash
./relay [options]

Options:
  -f string
        Configuration file path (default "/etc/relay/config.yml")
  -t    Output configuration template and exit
  -h    Show help message
```

### Running with Custom Configuration

```bash
# Using custom config file
./relay -f ./my-config.yml

# Generate template
./relay -t > my-config.yml
```

### Running Directly with Go

```bash
go run main.go -f config.yml
```

## Architecture

### Data Flow

1. **TCP Listener**: Accepts connections on the configured port (default 9514)
2. **Data Processing**: Incoming data is parsed as JSON (falls back to raw text)
3. **Event Wrapping**: Data is wrapped in Splunk event format with metadata
4. **Batching**: Events are queued and batched based on size or timeout
5. **Splunk Delivery**: Batched events are sent to Splunk HEC via HTTP POST

### Event Format

Events sent to Splunk follow this structure:

```json
{
  "time": 1641234567,
  "host": "relay-server",
  "source": "tcp:9514",
  "sourcetype": "zscaler:zpa:lss",
  "index": "zscaler",
  "event": { /* original log data */ }
}
```

## Monitoring and Logging

The service logs to stdout and includes:
- Connection status messages
- Batch processing information
- Error conditions
- Graceful shutdown notifications

## Development

### Prerequisites

- Go 1.24.4+
- Task runner (optional)

### Building

```bash
# Using Go directly
go build -o relay main.go

# Using Task
task build
```

### Running Tests

```bash
go test -v
```

### Dependencies

- `gopkg.in/yaml.v3` - YAML configuration parsing
- Go standard library only

## Deployment

### Systemd Service

Create `/etc/systemd/system/relay.service`:

```ini
[Unit]
Description=Zscaler ZPA LSS Relay Service
After=network.target

[Service]
Type=simple
User=relay
ExecStart=/usr/local/bin/relay -f /etc/relay/config.yml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable relay
sudo systemctl start relay
```

### Docker

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o relay main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/relay .
COPY config.yml .
CMD ["./relay", "-f", "config.yml"]
```

## Troubleshooting

### Common Issues

1. **Connection Refused**: Ensure the listen port is not in use and firewall allows connections
2. **Splunk HEC Errors**: Verify HEC URL and token are correct
3. **Permission Denied**: Check file permissions for configuration file
4. **Memory Usage**: Monitor batch size settings for high-volume environments

### Logs

Check service logs for diagnostic information:

```bash
# Systemd
journalctl -u relay -f

# Docker
docker logs <container_id>
```

## Performance Tuning

- **Batch Size**: Increase `batch_size` for higher throughput, decrease for lower latency
- **Batch Timeout**: Balance between latency and efficiency
- **System Resources**: Monitor CPU and memory usage under load

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

For issues and questions:
- Create an issue in the GitHub repository
- Check the troubleshooting section above
- Review logs for error messages
