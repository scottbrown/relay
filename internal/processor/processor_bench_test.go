package processor

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// BenchmarkReadLineLimited_Small benchmarks reading small lines (100 bytes)
func BenchmarkReadLineLimited_Small(b *testing.B) {
	line := strings.Repeat("a", 100) + "\n"
	data := bytes.Repeat([]byte(line), 1000)

	b.SetBytes(int64(len(line)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		br := bufio.NewReader(bytes.NewReader(data))
		_, err := ReadLineLimited(br, 1024*1024)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadLineLimited_Medium benchmarks reading medium lines (1KB)
func BenchmarkReadLineLimited_Medium(b *testing.B) {
	line := strings.Repeat("a", 1024) + "\n"
	data := bytes.Repeat([]byte(line), 1000)

	b.SetBytes(int64(len(line)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		br := bufio.NewReader(bytes.NewReader(data))
		_, err := ReadLineLimited(br, 1024*1024)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadLineLimited_Large benchmarks reading large lines (10KB)
func BenchmarkReadLineLimited_Large(b *testing.B) {
	line := strings.Repeat("a", 10*1024) + "\n"
	data := bytes.Repeat([]byte(line), 100)

	b.SetBytes(int64(len(line)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		br := bufio.NewReader(bytes.NewReader(data))
		_, err := ReadLineLimited(br, 1024*1024)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadLineLimited_MaxSize benchmarks reading maximum-sized lines (1MB - newline)
func BenchmarkReadLineLimited_MaxSize(b *testing.B) {
	// Create a line that's exactly at the limit (1MB minus newline)
	line := strings.Repeat("a", 1024*1024-1) + "\n"

	b.SetBytes(int64(len(line)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		br := bufio.NewReader(bytes.NewReader([]byte(line)))
		_, err := ReadLineLimited(br, 1024*1024)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadLineLimited_Oversized benchmarks behaviour with oversized lines
func BenchmarkReadLineLimited_Oversized(b *testing.B) {
	line := strings.Repeat("a", 2*1024*1024) + "\n"

	b.SetBytes(int64(len(line)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		br := bufio.NewReader(bytes.NewReader([]byte(line)))
		_, err := ReadLineLimited(br, 1024*1024)
		if err == nil {
			b.Fatal("expected error for oversized line")
		}
	}
}

// BenchmarkIsValidJSON_Small benchmarks JSON validation of small payloads (100 bytes)
func BenchmarkIsValidJSON_Small(b *testing.B) {
	data := []byte(`{"timestamp":"2024-01-01T00:00:00Z","event":"test","level":"info"}`)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !IsValidJSON(data) {
			b.Fatal("expected valid JSON")
		}
	}
}

// BenchmarkIsValidJSON_Medium benchmarks JSON validation of medium payloads (1KB)
func BenchmarkIsValidJSON_Medium(b *testing.B) {
	var sb strings.Builder
	sb.WriteString(`{"timestamp":"2024-01-01T00:00:00Z","event":"test","data":"`)
	sb.WriteString(strings.Repeat("x", 900))
	sb.WriteString(`"}`)
	data := []byte(sb.String())

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !IsValidJSON(data) {
			b.Fatal("expected valid JSON")
		}
	}
}

// BenchmarkIsValidJSON_Large benchmarks JSON validation of large payloads (10KB)
func BenchmarkIsValidJSON_Large(b *testing.B) {
	var sb strings.Builder
	sb.WriteString(`{"timestamp":"2024-01-01T00:00:00Z","event":"test","data":"`)
	sb.WriteString(strings.Repeat("x", 10*1024-100))
	sb.WriteString(`"}`)
	data := []byte(sb.String())

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !IsValidJSON(data) {
			b.Fatal("expected valid JSON")
		}
	}
}

// BenchmarkIsValidJSON_Invalid benchmarks JSON validation of invalid data
func BenchmarkIsValidJSON_Invalid(b *testing.B) {
	data := []byte(`{"timestamp":"2024-01-01T00:00:00Z","event":"test"`)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if IsValidJSON(data) {
			b.Fatal("expected invalid JSON")
		}
	}
}

// BenchmarkTruncate benchmarks string truncation
func BenchmarkTruncate(b *testing.B) {
	data := bytes.Repeat([]byte("a"), 1000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Truncate(data, 200)
	}
}
