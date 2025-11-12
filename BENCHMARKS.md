# Performance Benchmarks

This document contains baseline performance metrics for the relay service, measured on multiple Apple Silicon machines (darwin/arm64).

## Running Benchmarks

```bash
# Run all benchmarks
task bench

# Run specific package benchmarks
go test -bench=. -benchmem ./internal/processor/
go test -bench=. -benchmem ./internal/storage/
go test -bench=. -benchmem ./internal/forwarder/
go test -bench=. -benchmem ./internal/server/
```

## Baseline Performance

### Apple M1 Pro (2025-11-10)

**Test Environment:**
- CPU: Apple M1 Pro
- OS: darwin/arm64
- Go version: 1.21+

### Processor Package

Line reading performance with various payload sizes:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| ReadLineLimited_Small (100B) | 1,980,716 | 609.5 ns | 165.71 MB/s | 4,368 B | 4 |
| ReadLineLimited_Medium (1KB) | 1,230,699 | 969.8 ns | 1056.94 MB/s | 6,448 B | 4 |
| ReadLineLimited_Large (10KB) | 278,532 | 4,265 ns | 2401.04 MB/s | 34,168 B | 8 |
| ReadLineLimited_MaxSize (1MB) | 3,230 | 343,008 ns | 3057.00 MB/s | 4,210,089 B | 269 |
| ReadLineLimited_Oversized (2MB) | 1,731 | 672,316 ns | 3119.30 MB/s | 8,449,458 B | 528 |

JSON validation performance:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| IsValidJSON_Small (66B) | 6,212,329 | 192.7 ns | 342.48 MB/s | 0 B | 0 |
| IsValidJSON_Medium (1KB) | 553,296 | 2,184 ns | 439.98 MB/s | 0 B | 0 |
| IsValidJSON_Large (10KB) | 53,134 | 22,534 ns | 452.70 MB/s | 0 B | 0 |
| IsValidJSON_Invalid | 7,106,104 | 168.4 ns | 296.83 MB/s | 24 B | 1 |
| Truncate | 7,853,408 | 153.2 ns | N/A | 1,232 B | 2 |

**Key Observations:**
- JSON validation has zero allocations for valid payloads
- Line reading throughput scales well with payload size
- Small overhead for invalid JSON validation (24 B allocation)

### Storage Package

File write operations with daily rotation:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| Write_Small (100B) | 675,603 | 3,107 ns | 32.19 MB/s | 16 B | 1 |
| Write_Medium (1KB) | 438,000 | 2,685 ns | 381.45 MB/s | 1,552 B | 2 |
| Write_Large (10KB) | 133,090 | 8,455 ns | 1211.16 MB/s | 13,584 B | 2 |
| Write_Concurrent | 302,584 | 3,441 ns | 297.57 MB/s | 1,552 B | 2 |
| Rotation | 249 | 4,375,607 ns | N/A | 2,272 B | 10 |
| CurrentFile | 4,202,024 | 277.6 ns | N/A | 136 B | 2 |
| EnsureDir | 610,702 | 1,941 ns | N/A | 400 B | 3 |

**Key Observations:**
- Write throughput scales excellently with payload size (32 MB/s → 1211 MB/s)
- Minimal memory allocations for writes (1-2 allocs)
- File rotation is relatively expensive (~4.4ms) but infrequent
- Concurrent writes maintain good performance

### Forwarder Package

Splunk HEC forwarding performance:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| Forward_Small_NoGzip | 24,562 | 47,237 ns | 2.12 MB/s | 7,243 B | 85 |
| Forward_Small_Gzip | 8,881 | 136,389 ns | 0.73 MB/s | 842,102 B | 115 |
| Forward_Medium_NoGzip | 24,540 | 46,814 ns | 21.87 MB/s | 7,291 B | 85 |
| Forward_Medium_Gzip | 8,653 | 145,024 ns | 7.06 MB/s | 841,832 B | 115 |
| Forward_Large_NoGzip | 22,353 | 52,586 ns | 194.73 MB/s | 14,017 B | 88 |
| Forward_Large_Gzip | 7,531 | 173,730 ns | 58.94 MB/s | 840,814 B | 114 |
| Forward_WithRetry | 2 | 752,894,188 ns | 0.00 MB/s | 104,996 B | 335 |

Gzip compression performance:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| GzipCompression_Small | 18,190 | 64,388 ns | 1.55 MB/s | 813,939 B | 20 |
| GzipCompression_Medium | 17,082 | 67,223 ns | 15.23 MB/s | 813,939 B | 20 |
| GzipCompression_Large | 13,790 | 85,334 ns | 120.00 MB/s | 813,940 B | 21 |

Health check performance:

| Benchmark | Iterations | Time/op | Memory/op | Allocs/op |
|-----------|------------|---------|-----------|-----------|
| HealthCheck | 26,000 | 45,877 ns | 6,453 B | 71 |

**Key Observations:**
- Gzip compression adds significant overhead (2-3x slower) but scales well
- Gzip uses ~814KB memory per compression operation
- Retry logic adds substantial latency (~753ms with retries)
- No-gzip forwarding has low memory overhead (~7-14KB)

### Server Package

End-to-end connection handling performance:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| HandleConnection_Small | 271 | 4,423,603 ns | 2.49 MB/s | 119,556 B | 1,074 |
| HandleConnection_Medium | 225 | 5,319,331 ns | 19.59 MB/s | 264,919 B | 1,253 |
| HandleConnection_WithGzip | 228 | 5,226,763 ns | 2.11 MB/s | 928,748 B | 1,211 |
| HandleConnection_MixedValid | 253 | 4,723,340 ns | 1.37 MB/s | 97,625 B | 887 |
| HandleConnection_NoForwarding | 1,956 | 611,689 ns | 18.03 MB/s | 12,753 B | 116 |

**Key Observations:**
- End-to-end handling includes storage + HEC forwarding overhead
- No-forwarding mode is ~7-8x faster (storage-only)
- Gzip adds significant memory overhead (~810KB per connection)
- Mixed valid/invalid JSON handled gracefully with minimal overhead

## Performance Insights

### Throughput Characteristics

1. **Line Processing**: Excellent throughput (165-3119 MB/s) with low allocations
2. **JSON Validation**: Very fast with zero allocations for valid JSON
3. **Storage**: Scales well with payload size (32-1211 MB/s)
4. **HEC Forwarding**: Network-bound, gzip trades CPU for bandwidth

### Memory Profile

1. **Processor**: Minimal allocations (0-4 per operation)
2. **Storage**: Very efficient (1-2 allocations per write)
3. **Forwarder**: Gzip compression uses ~814KB buffer
4. **Server**: End-to-end ~120KB per connection (without gzip)

### Optimization Opportunities

1. **Gzip Compression**: Consider buffer pooling to reduce allocations
2. **Server Async Forwarding**: Current implementation may benefit from batching
3. **Connection Pooling**: HTTP client connection reuse already enabled

### Apple M2 (2025-11-11)

**Test Environment:**
- CPU: Apple M2
- OS: darwin/arm64
- Go version: 1.21+

#### Processor Package

Line reading performance with various payload sizes:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| ReadLineLimited_Small (100B) | 2,400,175 | 533.6 ns | 189.30 MB/s | 4,368 B | 4 |
| ReadLineLimited_Medium (1KB) | 1,471,080 | 812.7 ns | 1261.31 MB/s | 6,448 B | 4 |
| ReadLineLimited_Large (10KB) | 341,968 | 3,589 ns | 2853.09 MB/s | 34,168 B | 8 |
| ReadLineLimited_MaxSize (1MB) | 4,045 | 321,836 ns | 3258.11 MB/s | 4,210,103 B | 269 |
| ReadLineLimited_Oversized (2MB) | 1,953 | 608,422 ns | 3446.87 MB/s | 8,449,465 B | 528 |

JSON validation performance:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| IsValidJSON_Small (66B) | 6,106,771 | 194.2 ns | 339.86 MB/s | 0 B | 0 |
| IsValidJSON_Medium (1KB) | 554,745 | 2,192 ns | 438.44 MB/s | 0 B | 0 |
| IsValidJSON_Large (10KB) | 52,251 | 22,858 ns | 446.29 MB/s | 0 B | 0 |
| IsValidJSON_Invalid | 7,177,077 | 167.7 ns | 298.24 MB/s | 24 B | 1 |
| Truncate | 9,212,223 | 128.8 ns | N/A | 1,232 B | 2 |

**Key Observations:**
- JSON validation maintains zero allocations for valid payloads
- Line reading throughput remains excellent and scales well
- Small decrease in time/op for invalid JSON validation (167.7 ns vs 168.4 ns)

#### Storage Package

File write operations with daily rotation:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| Write_Small (100B) | 649,660 | 1,546 ns | 64.68 MB/s | 32 B | 2 |
| Write_Medium (1KB) | 509,670 | 2,315 ns | 442.37 MB/s | 1,576 B | 4 |
| Write_Large (10KB) | 190,916 | 6,066 ns | 1688.14 MB/s | 13,608 B | 4 |
| Write_Concurrent | 376,027 | 2,999 ns | 341.42 MB/s | 1,576 B | 4 |
| Rotation | 456 | 2,688,669 ns | N/A | 2,296 B | 12 |
| CurrentFile | 6,469,957 | 183.9 ns | N/A | 136 B | 2 |
| EnsureDir | 714,373 | 1,740 ns | N/A | 400 B | 3 |

**Key Observations:**
- Write throughput scales excellently with payload size (64.68 MB/s → 1688.14 MB/s)
- Minimal memory allocations for writes (2-4 allocs)
- File rotation remains relatively expensive (~2.7ms) but infrequent
- Concurrent writes maintain excellent performance

#### Forwarder Package

Splunk HEC forwarding performance:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| Forward_Small_NoGzip | 28,137 | 41,793 ns | 2.39 MB/s | 7,337 B | 92 |
| Forward_Small_Gzip | 9,955 | 116,132 ns | 0.86 MB/s | 842,141 B | 122 |
| Forward_Medium_NoGzip | 28,694 | 41,741 ns | 24.53 MB/s | 7,385 B | 92 |
| Forward_Medium_Gzip | 10,000 | 116,050 ns | 8.82 MB/s | 842,007 B | 122 |
| Forward_Large_NoGzip | 27,445 | 43,429 ns | 235.79 MB/s | 14,144 B | 95 |
| Forward_Large_Gzip | 8,952 | 134,867 ns | 75.93 MB/s | 840,160 B | 121 |
| Forward_WithRetry | 2 | 752,810,375 ns | 0.00 MB/s | 70,224 B | 344 |

Gzip compression performance:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| GzipCompression_Small | 19,730 | 57,381 ns | 1.74 MB/s | 813,942 B | 20 |
| GzipCompression_Medium | 21,656 | 55,260 ns | 18.53 MB/s | 813,940 B | 20 |
| GzipCompression_Large | 16,734 | 70,890 ns | 144.45 MB/s | 814,051 B | 21 |

Health check performance:

| Benchmark | Iterations | Time/op | Memory/op | Allocs/op |
|-----------|------------|---------|-----------|-----------|
| HealthCheck | 30,115 | 39,150 ns | 6,449 B | 71 |

**Key Observations:**
- Gzip compression adds significant overhead but scales well
- Gzip maintains consistent ~814KB memory per compression operation
- Retry logic adds substantial latency (~753ms with retries)
- No-gzip forwarding has low memory overhead (~7-14KB)

#### Server Package

End-to-end connection handling performance:

| Benchmark | Iterations | Time/op | Throughput | Memory/op | Allocs/op |
|-----------|------------|---------|------------|-----------|-----------|
| HandleConnection_Small | 3,842 | 8,064,323 ns | 0.06 MB/s | 176,028 B | 1,647 |
| HandleConnection_Medium | 4,915 | 10,786,017 ns | 0.89 MB/s | 290,435 B | 2,368 |
| HandleConnection_WithGzip | 925 | 1,139,180 ns | 0.46 MB/s | 8,342,742 B | 2,139 |
| HandleConnection_MixedValid | 10,000 | 3,929,187 ns | 0.05 MB/s | 72,608 B | 648 |
| HandleConnection_NoForwarding | 36,446 | 32,002 ns | 16.25 MB/s | 9,571 B | 104 |

**Key Observations:**
- End-to-end handling includes storage + HEC forwarding overhead
- No-forwarding mode is significantly faster (storage-only)
- Gzip adds substantial memory overhead (~8.3MB per connection)
- Mixed valid/invalid JSON handled gracefully

## Tracking Performance

To track performance over time:

```bash
# Generate new baseline
go test -bench=. -benchmem ./internal/... > benchmarks-$(date +%Y-%m-%d).txt

# Compare with baseline
go install golang.org/x/perf/cmd/benchstat@latest
benchstat benchmarks-baseline.txt benchmarks-new.txt
```

## Continuous Monitoring

Consider adding benchmark CI checks to detect regressions:

```yaml
# Example GitHub Actions workflow
- name: Run Benchmarks
  run: task bench

- name: Compare with baseline
  run: benchstat baseline.txt current.txt
```
