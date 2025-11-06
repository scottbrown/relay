package main

var (
	configFile         string
	templateFlag       bool
	smokeTestFlag      bool
	listenAddr         string
	tlsCertFile        string
	tlsKeyFile         string
	outDir             string
	hecURL             string
	hecToken           string
	hecSourcetype      string
	allowedCIDRs       string
	gzipHEC            bool
	maxLineBytes       int
	healthCheckEnabled bool
	healthCheckAddr    string
)
