package storage

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// BenchmarkWrite_Small benchmarks writing small log lines (100 bytes)
func BenchmarkWrite_Small(b *testing.B) {
	tmpDir := b.TempDir()
	mgr, err := New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer mgr.Close()

	data := []byte(strings.Repeat("a", 100))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := mgr.Write(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWrite_Medium benchmarks writing medium log lines (1KB)
func BenchmarkWrite_Medium(b *testing.B) {
	tmpDir := b.TempDir()
	mgr, err := New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer mgr.Close()

	data := []byte(strings.Repeat("a", 1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := mgr.Write(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWrite_Large benchmarks writing large log lines (10KB)
func BenchmarkWrite_Large(b *testing.B) {
	tmpDir := b.TempDir()
	mgr, err := New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer mgr.Close()

	data := []byte(strings.Repeat("a", 10*1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := mgr.Write(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWrite_Concurrent benchmarks concurrent writes from multiple goroutines
func BenchmarkWrite_Concurrent(b *testing.B) {
	tmpDir := b.TempDir()
	mgr, err := New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer mgr.Close()

	data := []byte(strings.Repeat("a", 1024))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	var wg sync.WaitGroup
	numWorkers := 10
	writesPerWorker := b.N / numWorkers

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < writesPerWorker; i++ {
				if err := mgr.Write(data); err != nil {
					b.Error(err)
					return
				}
			}
		}()
	}

	wg.Wait()
}

// BenchmarkRotation benchmarks the file rotation logic
func BenchmarkRotation(b *testing.B) {
	tmpDir := b.TempDir()

	data := []byte(strings.Repeat("a", 1024))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create a new manager for each iteration to simulate rotation
		mgr, err := New(tmpDir, "bench")
		if err != nil {
			b.Fatal(err)
		}

		// Write a single line
		if err := mgr.Write(data); err != nil {
			mgr.Close()
			b.Fatal(err)
		}

		if err := mgr.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCurrentFile benchmarks getting the current file path
func BenchmarkCurrentFile(b *testing.B) {
	tmpDir := b.TempDir()
	mgr, err := New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer mgr.Close()

	// Write once to initialise the current file
	if err := mgr.Write([]byte("test")); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = mgr.CurrentFile()
	}
}

// BenchmarkEnsureDir benchmarks directory creation
func BenchmarkEnsureDir(b *testing.B) {
	tmpDir := b.TempDir()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dir := filepath.Join(tmpDir, "subdir", "nested")
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatal(err)
		}
	}
}
