package forwarder

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// BenchmarkForward_Small_NoGzip benchmarks forwarding small payloads (100 bytes) without gzip
func BenchmarkForward_Small_NoGzip(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hec := New(Config{
		URL:     server.URL,
		Token:   "test-token",
		UseGzip: false,
	})

	data := []byte(strings.Repeat("a", 100))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := hec.Forward(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkForward_Small_Gzip benchmarks forwarding small payloads (100 bytes) with gzip
func BenchmarkForward_Small_Gzip(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hec := New(Config{
		URL:     server.URL,
		Token:   "test-token",
		UseGzip: true,
	})

	data := []byte(strings.Repeat("a", 100))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := hec.Forward(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkForward_Medium_NoGzip benchmarks forwarding medium payloads (1KB) without gzip
func BenchmarkForward_Medium_NoGzip(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hec := New(Config{
		URL:     server.URL,
		Token:   "test-token",
		UseGzip: false,
	})

	data := []byte(strings.Repeat("a", 1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := hec.Forward(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkForward_Medium_Gzip benchmarks forwarding medium payloads (1KB) with gzip
func BenchmarkForward_Medium_Gzip(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hec := New(Config{
		URL:     server.URL,
		Token:   "test-token",
		UseGzip: true,
	})

	data := []byte(strings.Repeat("a", 1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := hec.Forward(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkForward_Large_NoGzip benchmarks forwarding large payloads (10KB) without gzip
func BenchmarkForward_Large_NoGzip(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hec := New(Config{
		URL:     server.URL,
		Token:   "test-token",
		UseGzip: false,
	})

	data := []byte(strings.Repeat("a", 10*1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := hec.Forward(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkForward_Large_Gzip benchmarks forwarding large payloads (10KB) with gzip
func BenchmarkForward_Large_Gzip(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hec := New(Config{
		URL:     server.URL,
		Token:   "test-token",
		UseGzip: true,
	})

	data := []byte(strings.Repeat("a", 10*1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := hec.Forward(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkForward_WithRetry benchmarks forwarding with retry logic
func BenchmarkForward_WithRetry(b *testing.B) {
	var failCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		// Fail first 2 attempts, succeed on 3rd
		if failCount.Add(1)%3 != 0 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	hec := New(Config{
		URL:     server.URL,
		Token:   "test-token",
		UseGzip: false,
	})

	data := []byte(strings.Repeat("a", 1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := hec.Forward(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGzipCompression benchmarks gzip compression alone
func BenchmarkGzipCompression_Small(b *testing.B) {
	data := []byte(strings.Repeat("a", 100))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf strings.Builder
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(data); err != nil {
			b.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGzipCompression_Medium benchmarks gzip compression of medium payloads
func BenchmarkGzipCompression_Medium(b *testing.B) {
	data := []byte(strings.Repeat("a", 1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf strings.Builder
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(data); err != nil {
			b.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGzipCompression_Large benchmarks gzip compression of large payloads
func BenchmarkGzipCompression_Large(b *testing.B) {
	data := []byte(strings.Repeat("a", 10*1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf strings.Builder
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(data); err != nil {
			b.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHealthCheck benchmarks the health check operation
func BenchmarkHealthCheck(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hec := New(Config{
		URL:   server.URL + "/services/collector/raw",
		Token: "test-token",
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := hec.HealthCheck(); err != nil {
			b.Fatal(err)
		}
	}
}
