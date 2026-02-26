package proxy

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"ztna-gateway/internal/config"
)

// newTestProxy crée un TCPProxy pour les tests.
func newTestProxy(dialTimeout string) *TCPProxy {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			DialTimeout: dialTimeout,
			MaxConns:    100,
		},
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return NewTCPProxy(cfg, log)
}

// startEchoServer démarre un serveur TCP qui recopie chaque octet reçu.
// Retourne l'adresse d'écoute, la fonction d'arrêt et une erreur éventuelle.
func startEchoServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen echo server : %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // Listener fermé
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) //nolint — echo
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// TestProxyBidirectional vérifie que le proxy relaie les données dans les deux sens.
func TestProxyBidirectional(t *testing.T) {
	// Démarrer un serveur echo sur loopback
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	host, portStr, _ := net.SplitHostPort(echoAddr)
	var port int
	if _, err := net.ResolveTCPAddr("tcp", echoAddr); err != nil {
		t.Fatalf("adresse echo invalide : %v", err)
	}
	// Extraire le port numériquement
	addr, _ := net.ResolveTCPAddr("tcp", echoAddr)
	port = addr.Port
	_ = host

	p := newTestProxy("5s")

	// Créer une paire de connexions (client ↔ proxy)
	clientSide, proxySide := net.Pipe()
	defer clientSide.Close()
	defer proxySide.Close()

	proxyDone := make(chan error, 1)
	go func() {
		proxyDone <- p.Proxy(context.Background(), proxySide, "127.0.0.1", port)
	}()

	// Envoyer des données depuis le côté client
	payload := []byte("hello-datavault-ztna")
	clientSide.SetDeadline(time.Now().Add(5 * time.Second)) //nolint
	if _, err := clientSide.Write(payload); err != nil {
		t.Fatalf("Write() vers proxy : %v", err)
	}

	// Lire l'écho retourné
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(clientSide, buf); err != nil {
		t.Fatalf("ReadFull() écho : %v", err)
	}
	if string(buf) != string(payload) {
		t.Errorf("écho = %q, attendu %q", buf, payload)
	}

	// Fermer le côté client → proxy doit se terminer
	clientSide.Close()
	select {
	case <-proxyDone:
	case <-time.After(2 * time.Second):
		t.Error("proxy n'a pas terminé après fermeture du client")
	}
	_ = portStr
}

// TestProxyTimeout vérifie que le proxy retourne une erreur pour un hôte injoignable.
func TestProxyTimeout(t *testing.T) {
	// Utiliser un port fermé sur loopback → connexion refusée immédiatement
	p := newTestProxy("2s")
	clientSide, proxySide := net.Pipe()
	defer clientSide.Close()
	defer proxySide.Close()

	// Port 19876 — très probablement fermé
	err := p.Proxy(context.Background(), proxySide, "127.0.0.1", 19876)
	if err == nil {
		t.Error("Proxy() doit retourner une erreur pour hôte/port injoignable")
	}
}

// TestProxyHalfClose vérifie que la fermeture d'un côté entraîne la fin du proxy.
func TestProxyHalfClose(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	addr, _ := net.ResolveTCPAddr("tcp", echoAddr)
	p := newTestProxy("5s")

	clientSide, proxySide := net.Pipe()
	defer clientSide.Close()
	defer proxySide.Close()

	done := make(chan error, 1)
	go func() {
		done <- p.Proxy(context.Background(), proxySide, "127.0.0.1", addr.Port)
	}()

	// Fermer le write côté client → envoie EOF au serveur echo
	clientSide.Close()

	select {
	case <-done:
		// Proxy terminé proprement
	case <-time.After(3 * time.Second):
		t.Error("Proxy() n'a pas terminé après half-close")
	}
}

// TestProxyBytesCounting vérifie que le proxy transfère correctement un volume connu de données.
func TestProxyBytesCounting(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	addr, _ := net.ResolveTCPAddr("tcp", echoAddr)
	p := newTestProxy("5s")

	clientSide, proxySide := net.Pipe()
	defer clientSide.Close()
	defer proxySide.Close()

	go func() {
		p.Proxy(context.Background(), proxySide, "127.0.0.1", addr.Port) //nolint
	}()

	const dataSize = 4096
	sent := make([]byte, dataSize)
	for i := range sent {
		sent[i] = byte(i % 256)
	}

	clientSide.SetDeadline(time.Now().Add(5 * time.Second)) //nolint
	clientSide.Write(sent)                                   //nolint

	received := make([]byte, dataSize)
	n, _ := io.ReadFull(clientSide, received)
	if n != dataSize {
		t.Errorf("octets reçus = %d, attendu %d", n, dataSize)
	}
	for i := 0; i < n; i++ {
		if received[i] != sent[i] {
			t.Errorf("données corrompues à l'octet %d : reçu %02x, attendu %02x", i, received[i], sent[i])
			break
		}
	}
}

// TestProxyContextCancellation vérifie que l'annulation du contexte coupe le proxy.
func TestProxyContextCancellation(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	addr, _ := net.ResolveTCPAddr("tcp", echoAddr)
	p := newTestProxy("5s")

	clientSide, proxySide := net.Pipe()
	defer clientSide.Close()
	defer proxySide.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- p.Proxy(ctx, proxySide, "127.0.0.1", addr.Port)
	}()

	// Laisser le proxy s'établir
	time.Sleep(50 * time.Millisecond)

	// Annuler le contexte → doit couper la connexion
	cancel()

	select {
	case <-done:
		// Proxy terminé correctement
	case <-time.After(2 * time.Second):
		t.Error("Proxy() n'a pas terminé après annulation du contexte")
	}
}

// TestProxyRateLimit vérifie que le proxy fonctionne même sans rate limit (rate_limit = 0).
func TestProxyRateLimit(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	addr, _ := net.ResolveTCPAddr("tcp", echoAddr)
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			DialTimeout: "5s",
			MaxConns:    100,
			RateLimit:   0, // 0 = pas de limite (comportement lab)
		},
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	p := NewTCPProxy(cfg, log)

	// Lancer 3 proxies simultanés → tous doivent fonctionner
	for i := 0; i < 3; i++ {
		clientSide, proxySide := net.Pipe()
		done := make(chan error, 1)
		go func(c, ps net.Conn) {
			defer c.Close()
			defer ps.Close()
			done <- p.Proxy(context.Background(), ps, "127.0.0.1", addr.Port)
		}(clientSide, proxySide)

		clientSide.SetDeadline(time.Now().Add(3 * time.Second)) //nolint
		clientSide.Write([]byte("ping"))                         //nolint

		buf := make([]byte, 4)
		io.ReadFull(clientSide, buf) //nolint
		clientSide.Close()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("proxy %d n'a pas terminé", i+1)
		}
	}
}
