# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a TCP relay service that receives Zscaler ZPA LSS (Log Streaming Service) data and forwards it to Splunk HEC (HTTP Event Collector). The application acts as a middleware layer that batches log events for efficient delivery to Splunk.

## Build and Development Commands

- `task build` - Build the binary to `.build/relay`
- `go run main.go` - Run the application directly
- `go build -o .build/relay github.com/scottbrown/relay` - Manual build command

## Application Architecture

### Core Components

The entire application is contained in `main.go` with these key components:

1. **Config Management** (`Config` struct): Handles YAML configuration loading with fields for Splunk HEC URL, authentication token, batch settings, and networking configuration.

2. **TCP Server**: Listens on a configurable port (default 9514) and accepts incoming connections from Zscaler ZPA LSS.

3. **BatchProcessor**: Manages event batching with configurable batch size and timeout. Events are collected and sent to Splunk HEC in batches for efficiency.

4. **Splunk HEC Integration**: Formats events as JSON and sends them via HTTP POST to Splunk's HEC endpoint.

### Data Flow

1. TCP connections accepted on listen port
2. Incoming data parsed as JSON (falls back to raw text if parsing fails)
3. Events wrapped in SplunkEvent format with metadata (timestamp, host, source, etc.)
4. Events added to BatchProcessor queue
5. BatchProcessor batches events based on size (`batch_size`) or timeout (`batch_timeout`)
6. Batched events sent to Splunk HEC via HTTP

### Configuration

The application supports two configuration methods:
- YAML config file (loaded via `-f` flag, defaults to `/etc/relay/config.yml`)
- Command-line flags (see `parseFlags()` function)

Required configuration: `splunk_hec_url` and `splunk_token`

### Template Generation

The `-t` flag outputs a configuration template using Go's embed feature. The template is embedded from `config.template.yml` at compile time.

## Dependencies

- `gopkg.in/yaml.v3` - YAML configuration parsing
- Standard library only (no external runtime dependencies)