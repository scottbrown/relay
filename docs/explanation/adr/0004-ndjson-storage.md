# ADR-0004: NDJSON for Local Storage

## Status

Accepted

## Context

The relay service needs to persist incoming log data to local disk for durability and potential replay. We need a file format that is:
- Simple to write (append-only)
- Simple to read and replay
- Human-readable for debugging
- Compatible with common tools
- Efficient for line-by-line processing

Options considered:
1. Plain text (one JSON per line, no validation)
2. NDJSON (Newline Delimited JSON)
3. JSON array (complete JSON document)
4. Binary format (Protocol Buffers, MessagePack)
5. SQLite database

## Decision

We will use NDJSON (Newline Delimited JSON) format for local storage. Each log entry is a complete JSON object on a single line, with lines separated by newlines (`\n`).

Format: `zpa-YYYY-MM-DD.ndjson`

## Consequences

### Positive

- **Simple append-only writes**: Just write JSON + newline, no complex file format
- **Line-oriented processing**: Easy to process with standard Unix tools (grep, awk, wc, etc.)
- **Tool compatibility**: Works with jq, Splunk, ELK, and other log processors
- **Human-readable**: Can inspect files with text editors or less/more
- **Streaming friendly**: Can read and process line-by-line without loading entire file
- **No schema required**: Each line is self-describing JSON
- **Crash-safe**: Partial writes only corrupt one line, file remains processable
- **Standard format**: NDJSON is a well-established format (RFC 7464 inspired)

### Negative

- **No compression**: Raw JSON can be verbose (mitigated with optional compression in retention policy)
- **No indexing**: Must scan entire file to find specific entries
- **Limited querying**: Can't query structured data without parsing every line
- **Larger than binary**: JSON is less space-efficient than binary formats

### Neutral

- Can add compression layer (gzip) without changing format
- Can build indices separately if needed
- Compatible with future database ingestion if requirements change
