package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/scottbrown/relay/internal/config"
)

type SplunkEvent struct {
	Time       int64       `json:"time"`
	Host       string      `json:"host"`
	Source     string      `json:"source"`
	SourceType string      `json:"sourcetype"`
	Index      string      `json:"index"`
	Event      interface{} `json:"event"`
}

type LogBatch struct {
	events []SplunkEvent
	timer  *time.Timer
}

func main() {
	var showTemplate bool
	flag.BoolVar(&showTemplate, "t", false, "Output configuration template and exit")

	if len(os.Args) > 1 && (os.Args[1] == "-t" || os.Args[1] == "--t") {
		fmt.Print(config.GetTemplate())
		os.Exit(0)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting Relay - ZPA LSS to Splunk HEC")
	log.Printf("Listening on port: %s", cfg.ListenPort)
	log.Printf("Forwarding to Splunk HEC: %s", cfg.SplunkHECURL)

	// Create HTTP client for Splunk HEC
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create batch processor
	batchProcessor := NewBatchProcessor(cfg, httpClient)
	go batchProcessor.Start()

	// Start TCP server
	listener, err := net.Listen("tcp", ":"+cfg.ListenPort)
	if err != nil {
		log.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		listener.Close()
		batchProcessor.Stop()
		os.Exit(0)
	}()

	// Accept connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go handleConnection(conn, cfg, batchProcessor)
	}
}

func parseFlags() *config.Config {
	cfg := &config.Config{}

	flag.StringVar(&cfg.ListenPort, "port", "9514", "Port to listen on")
	flag.StringVar(&cfg.SplunkHECURL, "hec-url", "", "Splunk HEC URL (required)")
	flag.StringVar(&cfg.SplunkToken, "token", "", "Splunk HEC token (required)")
	flag.StringVar(&cfg.SourceType, "sourcetype", "zscaler:zpa:lss", "Splunk sourcetype")
	flag.StringVar(&cfg.Index, "index", "main", "Splunk index")
	flag.IntVar(&cfg.BatchSize, "batch-size", 100, "Batch size for HEC submissions")
	flag.DurationVar(&cfg.BatchTimeout, "batch-timeout", 5*time.Second, "Batch timeout")

	flag.Parse()

	if cfg.SplunkHECURL == "" || cfg.SplunkToken == "" {
		fmt.Println("Usage: relay -hec-url <URL> -token <TOKEN> [options]")
		fmt.Println("\nRequired:")
		fmt.Println("  -hec-url    Splunk HEC URL (e.g., https://your-instance.splunkcloud.com:8088/services/collector)")
		fmt.Println("  -token      Splunk HEC token")
		fmt.Println("\nOptional:")
		fmt.Println("  -port       Listen port (default: 9514)")
		fmt.Println("  -sourcetype Splunk sourcetype (default: zscaler:zpa:lss)")
		fmt.Println("  -index      Splunk index (default: main)")
		fmt.Println("  -batch-size Batch size for HEC (default: 100)")
		fmt.Println("  -batch-timeout Batch timeout (default: 5s)")
		os.Exit(1)
	}

	return cfg
}

func handleConnection(conn net.Conn, cfg *config.Config, batchProcessor *BatchProcessor) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	log.Printf("New connection from: %s", clientAddr)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse JSON log entry
		var logData interface{}
		if err := json.Unmarshal([]byte(line), &logData); err != nil {
			log.Printf("Failed to parse JSON from %s: %v", clientAddr, err)
			// Send as raw text if JSON parsing fails
			logData = line
		}

		// Create Splunk event
		event := SplunkEvent{
			Time:       time.Now().Unix(),
			Host:       clientAddr,
			Source:     "zpa_lss",
			SourceType: cfg.SourceType,
			Index:      cfg.Index,
			Event:      logData,
		}

		// Send to batch processor
		batchProcessor.AddEvent(event)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Connection error from %s: %v", clientAddr, err)
	}

	log.Printf("Connection closed: %s", clientAddr)
}

type BatchProcessor struct {
	config     *config.Config
	httpClient *http.Client
	eventChan  chan SplunkEvent
	batch      *LogBatch
	stopChan   chan bool
}

func NewBatchProcessor(cfg *config.Config, httpClient *http.Client) *BatchProcessor {
	return &BatchProcessor{
		config:     cfg,
		httpClient: httpClient,
		eventChan:  make(chan SplunkEvent, 1000),
		batch:      &LogBatch{events: make([]SplunkEvent, 0, cfg.BatchSize)},
		stopChan:   make(chan bool),
	}
}

func (bp *BatchProcessor) Start() {
	for {
		select {
		case event := <-bp.eventChan:
			bp.addToBatch(event)
		case <-bp.stopChan:
			bp.flushBatch()
			return
		}
	}
}

func (bp *BatchProcessor) Stop() {
	bp.stopChan <- true
}

func (bp *BatchProcessor) AddEvent(event SplunkEvent) {
	select {
	case bp.eventChan <- event:
	default:
		log.Printf("Event channel full, dropping event")
	}
}

func (bp *BatchProcessor) addToBatch(event SplunkEvent) {
	bp.batch.events = append(bp.batch.events, event)

	// Start timer for first event in batch
	if len(bp.batch.events) == 1 {
		bp.batch.timer = time.AfterFunc(bp.config.BatchTimeout, func() {
			bp.flushBatch()
		})
	}

	// Flush if batch is full
	if len(bp.batch.events) >= bp.config.BatchSize {
		bp.flushBatch()
	}
}

func (bp *BatchProcessor) flushBatch() {
	if len(bp.batch.events) == 0 {
		return
	}

	// Stop timer if running
	if bp.batch.timer != nil {
		bp.batch.timer.Stop()
	}

	// Send to Splunk HEC
	if err := bp.sendToSplunk(bp.batch.events); err != nil {
		log.Printf("Failed to send batch to Splunk: %v", err)
	} else {
		log.Printf("Sent batch of %d events to Splunk", len(bp.batch.events))
	}

	// Reset batch
	bp.batch.events = bp.batch.events[:0]
	bp.batch.timer = nil
}

func (bp *BatchProcessor) sendToSplunk(events []SplunkEvent) error {
	// Convert events to JSON
	var buffer bytes.Buffer
	for _, event := range events {
		eventJSON, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal event: %v", err)
		}
		buffer.Write(eventJSON)
		buffer.WriteString("\n")
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", bp.config.SplunkHECURL, &buffer)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Splunk "+bp.config.SplunkToken)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := bp.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HEC returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
