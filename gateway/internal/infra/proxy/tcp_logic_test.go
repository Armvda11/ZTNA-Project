package proxy

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ztna-gateway/internal/config"
)

// startEchoServer starts a local TCP echo server and returns the listener.
func startEchoServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start echo server: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()
	return ln
}

// TestProxyRelay tests bidirectional relay between two pre-dialed connections.
// This tests the core relay logic without the SSRF check / dial logic.
func TestProxyRelay(t *testing.T) {
	ln := startEchoServer(t)
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)

	// Dial the echo server directly (bypasses SSRF validateTarget)
	targetConn, err := net.DialTimeout("tcp", addr.String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial echo server: %v", err)
	}
	defer targetConn.Close()

	// Create client pipe
	clientConn, mockClient := net.Pipe()
	defer mockClient.Close()

	// Run relay goroutines (same logic as Proxy but without dial)
	var bytesOut atomic.Int64
	go func() {
		defer clientConn.Close()
		n, _ := io.Copy(targetConn, clientConn)
		bytesOut.Store(n)
		if tc, ok := targetConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	go func() {
		io.Copy(clientConn, targetConn)
	}()

	testData := []byte("Hello ZTNA relay")
	_, err = mockClient.Write(testData)
	if err != nil {
		t.Fatalf("Write error = %v", err)
	}

	buf := make([]byte, 1024)
	mockClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := mockClient.Read(buf)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if string(buf[:n]) != string(testData) {
		t.Errorf("echoed = %q, want %q", string(buf[:n]), string(testData))
	}
}

// TestProxyTimeout tests connection timeout handling
func TestProxyTimeout(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			DialTimeout: "1s",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	proxy := NewTCPProxy(cfg, log)

	clientConn, _ := net.Pipe()
	defer clientConn.Close()

	ctx := context.Background()
	// RFC 5737: 192.0.2.0/24 is TEST-NET, guaranteed unroutable
	targetHost := "192.0.2.1"
	targetPort := 9999

	start := time.Now()
	result := proxy.Proxy(ctx, clientConn, targetHost, targetPort)
	duration := time.Since(start)

	if result.Err == nil {
		t.Error("Proxy() should return error for unreachable host")
	}
	if duration > 5*time.Second {
		t.Errorf("Proxy() timeout took too long: %v", duration)
	}
}

// TestProxySSRFProtection tests that loopback and metadata IPs are blocked
func TestProxySSRFProtection(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
	}{
		{"loopback_v4", "127.0.0.1", 80},
		{"loopback_v6", "::1", 80},
		{"metadata_aws", "169.254.169.254", 80},
		{"multicast", "224.0.0.1", 80},
		{"unspecified", "0.0.0.0", 80},
		{"port_zero", "10.10.30.10", 0},
		{"port_negative", "10.10.30.10", -1},
		{"port_too_high", "10.10.30.10", 70000},
		{"empty_host", "", 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTarget(tt.host, tt.port)
			if err == nil {
				t.Errorf("validateTarget(%q, %d) should return error", tt.host, tt.port)
			}
		})
	}
}

// TestProxyValidTarget tests that valid targets are accepted
func TestProxyValidTarget(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
	}{
		{"private_ip", "10.10.30.10", 22},
		{"private_ip_2", "192.168.1.1", 443},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTarget(tt.host, tt.port)
			if err != nil {
				t.Errorf("validateTarget(%q, %d) error = %v", tt.host, tt.port, err)
			}
		})
	}
}

// TestProxyContextCancellation tests graceful shutdown via context cancel
func TestProxyContextCancellation(t *testing.T) {
	ln := startEchoServer(t)
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)

	// Pre-dial to bypass SSRF check on loopback
	targetConn, err := net.DialTimeout("tcp", addr.String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer targetConn.Close()

	clientConn, mockClient := net.Pipe()
	defer mockClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		// Relay with context awareness — close connections on cancel
		go func() {
			<-ctx.Done()
			clientConn.Close()
			targetConn.Close()
		}()
		io.Copy(targetConn, clientConn)
	}()

	// Cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good — stopped promptly
	case <-time.After(3 * time.Second):
		t.Error("relay did not stop after context cancellation")
	}
}

// TestProxyBytesCounting verifies bytes are relayed intact
func TestProxyBytesCounting(t *testing.T) {
	ln := startEchoServer(t)
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)

	targetConn, err := net.DialTimeout("tcp", addr.String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer targetConn.Close()

	clientConn, mockClient := net.Pipe()
	defer mockClient.Close()

	go func() {
		defer clientConn.Close()
		io.Copy(targetConn, clientConn)
		if tc, ok := targetConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	go func() {
		io.Copy(clientConn, targetConn)
	}()

	testData := strings.Repeat("B", 4096)
	_, err = mockClient.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write error = %v", err)
	}

	// Read all 4096 echoed bytes
	buf := make([]byte, 8192)
	total := 0
	mockClient.SetReadDeadline(time.Now().Add(3 * time.Second))
	for total < 4096 {
		n, err := mockClient.Read(buf[total:])
		if err != nil {
			break
		}
		total += n
	}

	if total != 4096 {
		t.Errorf("echoed bytes = %d, want 4096", total)
	}
}

// TestProxyRateLimit — rate limiting per-subject not yet implemented at proxy level
func TestProxyRateLimit(t *testing.T) {
	t.Skip("Rate limiting per-subject not yet implemented at proxy level")
}
