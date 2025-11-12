package forwarder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scottbrown/relay/internal/config"
)

func TestNewMulti(t *testing.T) {
	tests := []struct {
		name        string
		targets     []config.HECTarget
		mode        config.RoutingMode
		wantErr     bool
		errContains string
	}{
		{
			name: "valid single target",
			targets: []config.HECTarget{
				{
					Name:       "primary",
					HECURL:     "http://localhost:8088",
					HECToken:   "token1",
					SourceType: "test",
				},
			},
			mode:    config.RoutingModeAll,
			wantErr: false,
		},
		{
			name: "valid multiple targets",
			targets: []config.HECTarget{
				{
					Name:       "primary",
					HECURL:     "http://localhost:8088",
					HECToken:   "token1",
					SourceType: "test",
				},
				{
					Name:       "secondary",
					HECURL:     "http://localhost:8089",
					HECToken:   "token2",
					SourceType: "test",
				},
			},
			mode:    config.RoutingModePrimaryFailover,
			wantErr: false,
		},
		{
			name:        "no targets",
			targets:     []config.HECTarget{},
			mode:        config.RoutingModeAll,
			wantErr:     true,
			errContains: "at least one HEC target is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewMulti(tt.targets, tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMulti() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errContains != "" && err != nil && err.Error() != tt.errContains {
					t.Errorf("NewMulti() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}
			if got == nil {
				t.Error("NewMulti() returned nil")
			}
			if len(got.targets) != len(tt.targets) {
				t.Errorf("NewMulti() created %d targets, want %d", len(got.targets), len(tt.targets))
			}
		})
	}
}

func TestMultiHEC_ForwardAll(t *testing.T) {
	var count1, count2 atomic.Int32

	// Create two test servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	// Create multi-target forwarder with "all" mode
	targets := []config.HECTarget{
		{
			Name:       "target1",
			HECURL:     server1.URL,
			HECToken:   "token1",
			SourceType: "test",
		},
		{
			Name:       "target2",
			HECURL:     server2.URL,
			HECToken:   "token2",
			SourceType: "test",
		},
	}

	multi, err := NewMulti(targets, config.RoutingModeAll)
	if err != nil {
		t.Fatalf("NewMulti() failed: %v", err)
	}

	// Forward data
	data := []byte(`{"test": "data"}`)
	err = multi.Forward("test-conn", data)
	if err != nil {
		t.Errorf("Forward() failed: %v", err)
	}

	// Both targets should receive the data
	time.Sleep(100 * time.Millisecond) // Give goroutines time to complete
	if count1.Load() != 1 {
		t.Errorf("Target 1 received %d requests, want 1", count1.Load())
	}
	if count2.Load() != 1 {
		t.Errorf("Target 2 received %d requests, want 1", count2.Load())
	}
}

func TestMultiHEC_ForwardPrimaryFailover(t *testing.T) {
	var count1, count2 atomic.Int32
	var primaryFails atomic.Bool

	// Primary server that can fail
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1.Add(1)
		if primaryFails.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	// Secondary server (always succeeds)
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	// Create multi-target forwarder with "primary-failover" mode
	targets := []config.HECTarget{
		{
			Name:       "primary",
			HECURL:     server1.URL,
			HECToken:   "token1",
			SourceType: "test",
		},
		{
			Name:       "secondary",
			HECURL:     server2.URL,
			HECToken:   "token2",
			SourceType: "test",
		},
	}

	multi, err := NewMulti(targets, config.RoutingModePrimaryFailover)
	if err != nil {
		t.Fatalf("NewMulti() failed: %v", err)
	}

	// Test 1: Primary succeeds
	data := []byte(`{"test": "data"}`)
	err = multi.Forward("test-conn", data)
	if err != nil {
		t.Errorf("Forward() failed when primary should succeed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if count1.Load() != 1 {
		t.Errorf("Primary received %d requests, want 1", count1.Load())
	}
	if count2.Load() != 0 {
		t.Errorf("Secondary received %d requests, want 0", count2.Load())
	}

	// Test 2: Primary fails, secondary succeeds
	primaryFails.Store(true)
	err = multi.Forward("test-conn", data)
	if err != nil {
		t.Errorf("Forward() failed when secondary should succeed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	// Primary should be tried again (and fail)
	if count1.Load() < 2 {
		t.Errorf("Primary received %d requests, want at least 2", count1.Load())
	}
	// Secondary should now receive the request
	if count2.Load() != 1 {
		t.Errorf("Secondary received %d requests, want 1", count2.Load())
	}
}

func TestMultiHEC_ForwardRoundRobin(t *testing.T) {
	var count1, count2 atomic.Int32

	// Create two test servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	// Create multi-target forwarder with "round-robin" mode
	targets := []config.HECTarget{
		{
			Name:       "target1",
			HECURL:     server1.URL,
			HECToken:   "token1",
			SourceType: "test",
		},
		{
			Name:       "target2",
			HECURL:     server2.URL,
			HECToken:   "token2",
			SourceType: "test",
		},
	}

	multi, err := NewMulti(targets, config.RoutingModeRoundRobin)
	if err != nil {
		t.Fatalf("NewMulti() failed: %v", err)
	}

	// Forward multiple requests
	data := []byte(`{"test": "data"}`)
	for i := 0; i < 4; i++ {
		err = multi.Forward("test-conn", data)
		if err != nil {
			t.Errorf("Forward() failed on iteration %d: %v", i, err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Each target should receive 2 requests (round-robin distribution)
	if count1.Load() != 2 {
		t.Errorf("Target 1 received %d requests, want 2", count1.Load())
	}
	if count2.Load() != 2 {
		t.Errorf("Target 2 received %d requests, want 2", count2.Load())
	}
}

func TestMultiHEC_HealthCheck(t *testing.T) {
	// Create a test server that responds to health checks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/services/collector/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	targets := []config.HECTarget{
		{
			Name:       "target1",
			HECURL:     server.URL + "/services/collector/raw",
			HECToken:   "token1",
			SourceType: "test",
		},
	}

	multi, err := NewMulti(targets, config.RoutingModeAll)
	if err != nil {
		t.Fatalf("NewMulti() failed: %v", err)
	}

	// Health check should succeed
	err = multi.HealthCheck()
	if err != nil {
		t.Errorf("HealthCheck() failed: %v", err)
	}
}

func TestMultiHEC_Shutdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targets := []config.HECTarget{
		{
			Name:       "target1",
			HECURL:     server.URL,
			HECToken:   "token1",
			SourceType: "test",
		},
	}

	multi, err := NewMulti(targets, config.RoutingModeAll)
	if err != nil {
		t.Fatalf("NewMulti() failed: %v", err)
	}

	// Shutdown should complete successfully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = multi.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown() failed: %v", err)
	}
}

func TestMultiHEC_DefaultRoutingMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targets := []config.HECTarget{
		{
			Name:       "target1",
			HECURL:     server.URL,
			HECToken:   "token1",
			SourceType: "test",
		},
	}

	// Create with empty routing mode (should default to "all")
	multi, err := NewMulti(targets, "")
	if err != nil {
		t.Fatalf("NewMulti() failed: %v", err)
	}

	if multi.mode != config.RoutingModeAll {
		t.Errorf("Default routing mode = %v, want %v", multi.mode, config.RoutingModeAll)
	}
}
