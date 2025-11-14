package main

import (
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/scottbrown/relay"
	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/config"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/healthcheck"
	"github.com/scottbrown/relay/internal/server"
	"github.com/scottbrown/relay/internal/storage"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     relay.AppName,
	Short:   "TCP relay service for Zscaler ZPA LSS data to Splunk HEC",
	Long:    "A TCP relay service that receives Zscaler ZPA LSS data and forwards it to Splunk HEC with local persistence.",
	Version: relay.Version(),
	Run:     handleRootCmd,
}

func handleRootCmd(cmd *cobra.Command, args []string) {
	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Override config with CLI flags if provided
	applyFlagOverrides(cfg, cmd)

	// Initialize ACL
	aclList, err := acl.New(cfg.AllowedCIDRs)
	if err != nil {
		log.Fatalf("allow-cidrs: %v", err)
	}

	// Initialize storage
	storageManager, err := storage.New(cfg.OutputDir)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer storageManager.Close()

	// Initialize HEC forwarder
	hecForwarder := forwarder.New(forwarder.Config{
		URL:        cfg.SplunkHECURL,
		Token:      cfg.SplunkToken,
		SourceType: cfg.SourceType,
		UseGzip:    cfg.GzipHEC,
	})

	// Perform startup health check for Splunk HEC (if configured)
	if cfg.SplunkHECURL != "" && cfg.SplunkToken != "" {
		log.Printf("Testing Splunk HEC connectivity...")
		if err := hecForwarder.HealthCheck(); err != nil {
			log.Fatalf("Splunk HEC health check failed: %v", err)
		}
		log.Printf("Splunk HEC connectivity verified successfully")
	}

	// Initialize and start healthcheck server if enabled
	var healthSrv *healthcheck.Server
	defer func() {
		if healthSrv != nil {
			_ = healthSrv.Stop() // #nosec G104 - Error on healthcheck stop during cleanup is non-critical
		}
	}()
	if cfg.HealthCheckEnabled {
		healthSrv = startHealthcheckOrFail(cfg.HealthCheckAddr)
	}

	// Initialize and start server
	srv, err := server.New(server.Config{
		ListenAddr:   cfg.ListenAddr,
		TLSCertFile:  cfg.TLSCertFile,
		TLSKeyFile:   cfg.TLSKeyFile,
		MaxLineBytes: cfg.MaxLineBytes,
	}, aclList, storageManager, hecForwarder)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	for {
		select {
		case err := <-serverErrCh:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				log.Fatalf("server: %v", err)
			}
			return
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				newCfg, reloadErr := reloadConfigRuntime(cmd, cfg, srv, hecForwarder, &healthSrv)
				if reloadErr != nil {
					log.Printf("reload failed: %v", reloadErr)
					continue
				}
				cfg = newCfg
				log.Printf("configuration reloaded from %s", configFile)
			default:
				log.Printf("received %s, shutting down", sig)
				_ = srv.Stop() // #nosec G104 - Error on stop during shutdown is non-critical, process exits anyway
				return
			}
		}
	}
}

func reloadConfigRuntime(cmd *cobra.Command, currentCfg *config.Config, srv *server.Server, hecForwarder *forwarder.HEC, healthSrv **healthcheck.Server) (*config.Config, error) {
	if configFile == "" {
		return currentCfg, errors.New("no config file provided; cannot reload")
	}

	newCfg, err := config.LoadConfig(configFile)
	if err != nil {
		return currentCfg, err
	}

	applyFlagOverrides(newCfg, cmd)

	handleImmutableSetting("listen_addr", currentCfg.ListenAddr, newCfg.ListenAddr, func() {
		newCfg.ListenAddr = currentCfg.ListenAddr
	})
	handleImmutableSetting("tls_cert_file", currentCfg.TLSCertFile, newCfg.TLSCertFile, func() {
		newCfg.TLSCertFile = currentCfg.TLSCertFile
	})
	handleImmutableSetting("tls_key_file", currentCfg.TLSKeyFile, newCfg.TLSKeyFile, func() {
		newCfg.TLSKeyFile = currentCfg.TLSKeyFile
	})
	handleImmutableSetting("output_dir", currentCfg.OutputDir, newCfg.OutputDir, func() {
		newCfg.OutputDir = currentCfg.OutputDir
	})

	newACL, err := acl.New(newCfg.AllowedCIDRs)
	if err != nil {
		return currentCfg, err
	}
	srv.UpdateACL(newACL)
	srv.UpdateMaxLineBytes(newCfg.MaxLineBytes)

	hecForwarder.UpdateConfig(forwarder.Config{
		URL:        newCfg.SplunkHECURL,
		Token:      newCfg.SplunkToken,
		SourceType: newCfg.SourceType,
		UseGzip:    newCfg.GzipHEC,
	})

	if err := reconcileHealthcheck(currentCfg, newCfg, healthSrv); err != nil {
		return currentCfg, err
	}

	return newCfg, nil
}

func handleImmutableSetting(name, oldVal, newVal string, revert func()) {
	if oldVal != newVal {
		log.Printf("%s changed during reload but requires restart; keeping %q", name, oldVal)
		revert()
	}
}

func reconcileHealthcheck(currentCfg, newCfg *config.Config, healthSrv **healthcheck.Server) error {
	switch {
	case !currentCfg.HealthCheckEnabled && newCfg.HealthCheckEnabled:
		srv, err := startHealthcheck(newCfg.HealthCheckAddr)
		if err != nil {
			return err
		}
		*healthSrv = srv
		log.Printf("healthcheck enabled on %s", newCfg.HealthCheckAddr)
	case currentCfg.HealthCheckEnabled && !newCfg.HealthCheckEnabled:
		if *healthSrv != nil {
			if err := (*healthSrv).Stop(); err != nil {
				return err
			}
			*healthSrv = nil
		}
		log.Printf("healthcheck disabled")
	case currentCfg.HealthCheckEnabled && newCfg.HealthCheckEnabled && currentCfg.HealthCheckAddr != newCfg.HealthCheckAddr:
		if *healthSrv != nil {
			if err := (*healthSrv).Stop(); err != nil {
				return err
			}
		}
		srv, err := startHealthcheck(newCfg.HealthCheckAddr)
		if err != nil {
			return err
		}
		*healthSrv = srv
		log.Printf("healthcheck address updated to %s", newCfg.HealthCheckAddr)
	}
	return nil
}

func startHealthcheck(addr string) (*healthcheck.Server, error) {
	healthSrv, err := healthcheck.New(addr)
	if err != nil {
		return nil, err
	}
	if err := healthSrv.Start(); err != nil {
		return nil, err
	}
	return healthSrv, nil
}

func startHealthcheckOrFail(addr string) *healthcheck.Server {
	srv, err := startHealthcheck(addr)
	if err != nil {
		log.Fatalf("healthcheck: %v", err)
	}
	return srv
}
