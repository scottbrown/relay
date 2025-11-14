package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/scottbrown/relay/internal/config"
	"github.com/scottbrown/relay/internal/healthcheck"
)

func TestHandleImmutableSetting(t *testing.T) {
	tests := []struct {
		name           string
		settingName    string
		oldVal         string
		newVal         string
		expectRevert   bool
		expectedLogMsg string
	}{
		{
			name:         "values are same, no revert",
			settingName:  "listen_addr",
			oldVal:       ":9015",
			newVal:       ":9015",
			expectRevert: false,
		},
		{
			name:           "values differ, revert called",
			settingName:    "listen_addr",
			oldVal:         ":9015",
			newVal:         ":9016",
			expectRevert:   true,
			expectedLogMsg: "listen_addr changed during reload but requires restart; keeping \":9015\"",
		},
		{
			name:           "output_dir changed",
			settingName:    "output_dir",
			oldVal:         "./logs",
			newVal:         "./new-logs",
			expectRevert:   true,
			expectedLogMsg: "output_dir changed during reload but requires restart; keeping \"./logs\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var logBuf bytes.Buffer
			log.SetOutput(&logBuf)
			defer log.SetOutput(os.Stderr)

			reverted := false
			revertFunc := func() {
				reverted = true
			}

			handleImmutableSetting(tt.settingName, tt.oldVal, tt.newVal, revertFunc)

			if reverted != tt.expectRevert {
				t.Errorf("expected revert=%v, got %v", tt.expectRevert, reverted)
			}

			if tt.expectedLogMsg != "" {
				logOutput := logBuf.String()
				if !strings.Contains(logOutput, tt.expectedLogMsg) {
					t.Errorf("expected log to contain %q, got %q", tt.expectedLogMsg, logOutput)
				}
			}
		})
	}
}

func TestReconcileHealthcheck_EnableHealthcheck(t *testing.T) {
	currentCfg := &config.Config{
		HealthCheckEnabled: false,
	}

	newCfg := &config.Config{
		HealthCheckEnabled: true,
		HealthCheckAddr:    ":0", // Use :0 to get random available port
	}

	var healthSrv *healthcheck.Server

	// Capture log output
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	err := reconcileHealthcheck(currentCfg, newCfg, &healthSrv)
	if err != nil {
		t.Fatalf("reconcileHealthcheck should succeed: %v", err)
	}

	if healthSrv == nil {
		t.Fatal("healthcheck server should be created")
	}

	// Clean up
	if err := healthSrv.Stop(); err != nil {
		t.Errorf("failed to stop healthcheck server: %v", err)
	}

	// Verify log message
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "healthcheck enabled on") {
		t.Errorf("expected log to contain 'healthcheck enabled on', got %q", logOutput)
	}
}

func TestReconcileHealthcheck_DisableHealthcheck(t *testing.T) {
	// Start with health check enabled
	healthSrv, err := healthcheck.New(":0")
	if err != nil {
		t.Fatalf("failed to create healthcheck server: %v", err)
	}
	if err := healthSrv.Start(); err != nil {
		t.Fatalf("failed to start healthcheck server: %v", err)
	}

	currentCfg := &config.Config{
		HealthCheckEnabled: true,
		HealthCheckAddr:    ":9099",
	}

	newCfg := &config.Config{
		HealthCheckEnabled: false,
	}

	// Capture log output
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	err = reconcileHealthcheck(currentCfg, newCfg, &healthSrv)
	if err != nil {
		t.Fatalf("reconcileHealthcheck should succeed: %v", err)
	}

	if healthSrv != nil {
		t.Error("healthcheck server should be nil after disabling")
	}

	// Verify log message
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "healthcheck disabled") {
		t.Errorf("expected log to contain 'healthcheck disabled', got %q", logOutput)
	}
}

func TestReconcileHealthcheck_ChangeAddress(t *testing.T) {
	// Start with health check enabled on one address
	healthSrv, err := healthcheck.New(":0")
	if err != nil {
		t.Fatalf("failed to create healthcheck server: %v", err)
	}
	if err := healthSrv.Start(); err != nil {
		t.Fatalf("failed to start healthcheck server: %v", err)
	}

	currentCfg := &config.Config{
		HealthCheckEnabled: true,
		HealthCheckAddr:    ":9099",
	}

	newCfg := &config.Config{
		HealthCheckEnabled: true,
		HealthCheckAddr:    ":0", // Use :0 to get different random available port
	}

	// Capture log output
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	err = reconcileHealthcheck(currentCfg, newCfg, &healthSrv)
	if err != nil {
		t.Fatalf("reconcileHealthcheck should succeed: %v", err)
	}

	if healthSrv == nil {
		t.Fatal("healthcheck server should still be running on new address")
	}

	// Clean up
	if err := healthSrv.Stop(); err != nil {
		t.Errorf("failed to stop healthcheck server: %v", err)
	}

	// Verify log message
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "healthcheck address updated to") {
		t.Errorf("expected log to contain 'healthcheck address updated to', got %q", logOutput)
	}
}

func TestReconcileHealthcheck_NoChange(t *testing.T) {
	// Both configs have health check disabled
	currentCfg := &config.Config{
		HealthCheckEnabled: false,
	}

	newCfg := &config.Config{
		HealthCheckEnabled: false,
	}

	var healthSrv *healthcheck.Server

	err := reconcileHealthcheck(currentCfg, newCfg, &healthSrv)
	if err != nil {
		t.Fatalf("reconcileHealthcheck should succeed: %v", err)
	}

	if healthSrv != nil {
		t.Error("healthcheck server should remain nil")
	}
}

func TestReconcileHealthcheck_SameAddressEnabled(t *testing.T) {
	// Start with health check enabled
	healthSrv, err := healthcheck.New(":0")
	if err != nil {
		t.Fatalf("failed to create healthcheck server: %v", err)
	}
	if err := healthSrv.Start(); err != nil {
		t.Fatalf("failed to start healthcheck server: %v", err)
	}
	defer healthSrv.Stop()

	addr := ":9099"
	currentCfg := &config.Config{
		HealthCheckEnabled: true,
		HealthCheckAddr:    addr,
	}

	newCfg := &config.Config{
		HealthCheckEnabled: true,
		HealthCheckAddr:    addr, // Same address
	}

	// Capture log output
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	err = reconcileHealthcheck(currentCfg, newCfg, &healthSrv)
	if err != nil {
		t.Fatalf("reconcileHealthcheck should succeed: %v", err)
	}

	if healthSrv == nil {
		t.Error("healthcheck server should still be running")
	}

	// Verify no log messages about changes
	logOutput := logBuf.String()
	if strings.Contains(logOutput, "healthcheck enabled on") ||
		strings.Contains(logOutput, "healthcheck disabled") ||
		strings.Contains(logOutput, "healthcheck address updated to") {
		t.Errorf("expected no log messages for unchanged health check, got %q", logOutput)
	}
}
