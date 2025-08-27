package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	listenAddr   = flag.String("listen", ":9015", "TCP listen address (e.g., :9015)")
	tlsCertFile  = flag.String("tls-cert", "", "TLS cert file (optional)")
	tlsKeyFile   = flag.String("tls-key", "", "TLS key file (optional)")
	outDir       = flag.String("out", "./zpa-logs", "Directory to persist NDJSON")
	hecURL       = flag.String("hec-url", "", "Splunk HEC raw endpoint, e.g. https://splunk:8088/services/collector/raw")
	hecToken     = flag.String("hec-token", "", "Splunk HEC token")
	hecSourcetype= flag.String("hec-sourcetype", "zpa:lss", "Splunk sourcetype")
	allowedCIDRs = flag.String("allow-cidrs", "", "Comma-separated CIDRs allowed to connect (optional)")
	gzipHEC      = flag.Bool("hec-gzip", true, "Gzip compress payloads to HEC")
	maxLineBytes = flag.Int("max-line-bytes", 1<<20, "Max bytes per JSON line (default 1 MiB)")
)

type cidrList struct{ nets []*net.IPNet }
func (c *cidrList) allows(ip net.IP) bool {
	if len(c.nets) == 0 { return true }
	for _, n := range c.nets {
		if n.Contains(ip) { return true }
	}
	return false
}

func parseCIDRs(s string) (*cidrList, error) {
	if strings.TrimSpace(s) == "" { return &cidrList{}, nil }
	var res cidrList
	for _, part := range strings.Split(s, ",") {
		_, n, err := net.ParseCIDR(strings.TrimSpace(part))
		if err != nil { return nil, err }
		res.nets = append(res.nets, n)
	}
	return &res, nil
}

func ensureDir(d string) error { return os.MkdirAll(d, 0755) }

func openDayFile(base string) (*os.File, string, error) {
	day := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join(base, "zpa-"+day+".ndjson")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	return f, path, err
}

func main() {
	flag.Parse()
	if err := ensureDir(*outDir); err != nil { log.Fatalf("out dir: %v", err) }

	acl, err := parseCIDRs(*allowedCIDRs)
	if err != nil { log.Fatalf("allow-cidrs: %v", err) }

	var ln net.Listener
	if *tlsCertFile != "" && *tlsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(*tlsCertFile, *tlsKeyFile)
		if err != nil { log.Fatalf("load tls cert: %v", err) }
		cfg := &tls.Config{ Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12 }
		tln, err := tls.Listen("tcp", *listenAddr, cfg)
		if err != nil { log.Fatalf("listen tls: %v", err) }
		ln = tln
		log.Printf("listening TLS on %s", *listenAddr)
	} else {
		tln, err := net.Listen("tcp", *listenAddr)
		if err != nil { log.Fatalf("listen tcp: %v", err) }
		ln = tln
		log.Printf("listening TCP on %s", *listenAddr)
	}

	client := &http.Client{ Timeout: 15 * time.Second }

	for {
		conn, err := ln.Accept()
		if err != nil { log.Printf("accept: %v", err); continue }
		ra, _ := net.ResolveTCPAddr("tcp", conn.RemoteAddr().String())
		ip := ra.IP
		if !acl.allows(ip) {
			log.Printf("deny %s", ip)
			_ = conn.Close()
			continue
		}
		go handleConn(conn, client)
	}
}

func handleConn(conn net.Conn, client *http.Client) {
	defer conn.Close()
	br := bufio.NewReader(conn)

	// One file per day; reopen on rotation
	var curDay string
	var f *os.File
	defer func() { if f != nil { f.Close() } }()

	for {
		line, err := readLineLimited(br, *maxLineBytes)
		if errors.Is(err, io.EOF) { return }
		if err != nil { log.Printf("read: %v", err); return }
		// basic sanity: must be JSON object
		if !json.Valid(line) {
			log.Printf("invalid json from %s: %q", conn.RemoteAddr(), truncate(line, 200))
			continue
		}
		day := time.Now().UTC().Format("2006-01-02")
		if day != curDay {
			if f != nil { f.Close() }
			var path string
			var err error
			f, path, err = openDayFile(*outDir)
			if err != nil { log.Printf("open file: %v", err); return }
			curDay = day
			log.Printf("writing to %s", path)
		}
		// persist exactly as received
		if _, err := f.Write(append(line, '\n')); err != nil { log.Printf("write: %v", err) }

		// forward to Splunk HEC (optional)
		if *hecURL != "" && *hecToken != "" {
			if err := sendToHECRaw(client, line, *hecURL, *hecToken, *hecSourcetype, *gzipHEC); err != nil {
				log.Printf("hec: %v", err)
			}
		}
	}
}

func readLineLimited(br *bufio.Reader, limit int) ([]byte, error) {
	var buf bytes.Buffer
	for {
		b, err := br.ReadBytes('\n')
		buf.Write(b)
		if len(buf.Bytes()) > limit {
			return nil, errors.New("line exceeds limit")
		}
		if err != nil {
			if errors.Is(err, io.EOF) && buf.Len() > 0 { return bytes.TrimRight(buf.Bytes(), "\r\n"), nil }
			return nil, err
		}
		// got newline
		return bytes.TrimRight(buf.Bytes(), "\r\n"), nil
	}
}

func sendToHECRaw(client *http.Client, line []byte, url, token, sourcetype string, gz bool) error {
	var body io.Reader = bytes.NewReader(line)
	var contentEnc string
	var b bytes.Buffer
	if gz {
		zw := gzip.NewWriter(&b)
		if _, err := zw.Write(line); err != nil { return err }
		_ = zw.Close()
		body = &b
		contentEnc = "gzip"
	}
	req, err := http.NewRequest("POST", url, body)
	if err != nil { return err }
	req.Header.Set("Authorization", "Splunk "+token)
	req.Header.Set("Content-Type", "text/plain") // raw endpoint accepts text
	if contentEnc != "" { req.Header.Set("Content-Encoding", contentEnc) }
	// Add sourcetype/index in query if desired
	q := req.URL.Query()
	if sourcetype != "" { q.Set("sourcetype", sourcetype) }
	req.URL.RawQuery = q.Encode()

	// basic retry
	for i := 0; i < 5; i++ {
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = resp.Body.Close()
			return nil
		}
		if err == nil { io.Copy(io.Discard, resp.Body); resp.Body.Close() }
		time.Sleep(time.Duration(250* (1<<i)) * time.Millisecond)
	}
	return errors.New("hec send failed after retries")
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) <= n { return s }
	return s[:n] + "…"
}
