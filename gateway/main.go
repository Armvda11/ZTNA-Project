package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"

	"github.com/ztna/gateway/internal/certvalidator"
	"github.com/ztna/gateway/internal/config"
	"github.com/ztna/gateway/internal/controlplane"
	"github.com/ztna/gateway/internal/logger"
)

const (
	defaultConfigPath = "/etc/ztna/gateway-config.yaml"
	version           = "0.2.0"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", defaultConfigPath, "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ZTNA Gateway v%s\n", version)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.Logging)
	log.Info("Starting ZTNA Gateway", "version", version)

	// Initialize Control Plane client
	cpClient := controlplane.NewClient(cfg.ControlPlane, log)

	// Health check Control Plane
	log.Info("Checking Control Plane connectivity", "url", cfg.ControlPlane.URL)
	if err := cpClient.HealthCheck(); err != nil {
		log.Warn("Control Plane health check failed", "error", err)
		log.Warn("Continuing anyway, but policy enforcement may fail")
	} else {
		log.Info("Control Plane is reachable")
	}

	// Initialize certificate validator
	log.Info("Initializing certificate validator")
	caEndpoint := cfg.ControlPlane.CAPublicKeyEndpoint
	if caEndpoint == "" {
		caEndpoint = "/api/v1/ca/public-key"
		log.Warn("Control Plane CA endpoint not set, using default", "endpoint", caEndpoint)
	}
	certValidator, err := certvalidator.New(cpClient, caEndpoint, log)
	if err != nil {
		log.Error("Failed to initialize certificate validator", "error", err)
		log.Error("Certificate validation is REQUIRED for security")
		os.Exit(1)
	}
	log.Info("Certificate validator initialized", "ca_fingerprint", certValidator.GetCAFingerprint())

	// Load or generate host key
	hostKey, err := loadOrGenerateHostKey(cfg.SSH.HostKeyPath, log)
	if err != nil {
		log.Error("Failed to load/generate host key", "error", err)
		os.Exit(1)
	}
	log.Info("SSH host key loaded", "path", cfg.SSH.HostKeyPath)

	// Configure SSH server
	sshConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			// Validate SSH certificate
			result, err := certValidator.Validate(key, conn)
			if err != nil {
				log.Warn("Certificate validation failed",
					"user", conn.User(),
					"remote", conn.RemoteAddr().String(),
					"error", err)
				return nil, fmt.Errorf("certificate validation failed: %w", err)
			}

			// Marshal result to JSON for extensions
			resultJSON, _ := json.Marshal(result)

			log.Info("Certificate validated successfully",
				"user", conn.User(),
				"key_id", result.KeyID,
				"principals", result.Principals,
				"remote", conn.RemoteAddr().String())

			return &ssh.Permissions{
				Extensions: map[string]string{
					"cert_user":   result.Username,
					"principals":  fmt.Sprintf("%v", result.Principals),
					"key_id":      result.KeyID,
					"valid_until": result.ValidBefore.Format("2006-01-02 15:04:05"),
					"result_json": string(resultJSON),
				},
			}, nil
		},
	}

	sshConfig.AddHostKey(hostKey)

	// Start SSH server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.SSHPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error("Failed to listen on SSH port", "address", addr, "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	log.Info("SSH Gateway is ready", "address", addr)

	// Handle connections in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := listener.Accept()
				if err != nil {
					select {
					case <-ctx.Done():
						return
					default:
						log.Error("Failed to accept connection", "error", err)
						continue
					}
				}

				// Handle connection in goroutine
				go handleSSHConnection(conn, sshConfig, cfg, cpClient, log)
			}
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down gateway...")
	cancel()
	log.Info("Gateway exited gracefully")
}

func loadOrGenerateHostKey(path string, log *logger.Logger) (ssh.Signer, error) {
	// Try to load existing key
	keyData, err := os.ReadFile(path)
	if err == nil {
		signer, err := ssh.ParsePrivateKey(keyData)
		if err == nil {
			log.Info("Loaded existing host key", "path", path)
			return signer, nil
		}
		log.Warn("Failed to parse existing host key, generating new one", "error", err)
	}

	// Generate new Ed25519 key
	log.Info("Generating new SSH host key", "path", path)

	// TODO: Implement key generation (similar to Control Plane CA)
	// For now, return error - key must be provided
	return nil, fmt.Errorf("host key not found at %s - please generate with: ssh-keygen -t ed25519 -f %s -N ''", path, path)
}

func handleSSHConnection(netConn net.Conn, sshConfig *ssh.ServerConfig, cfg *config.Config, cpClient *controlplane.Client, log *logger.Logger) {
	defer netConn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(netConn, sshConfig)
	if err != nil {
		log.Error("SSH handshake failed", "remote", netConn.RemoteAddr(), "error", err)
		return
	}
	defer sshConn.Close()

	log.Info("SSH connection established",
		"user", sshConn.User(),
		"remote", sshConn.RemoteAddr().String(),
		"client_version", string(sshConn.ClientVersion()))

	// Discard global requests
	go ssh.DiscardRequests(reqs)

	// Handle channels
	for newChannel := range chans {
		log.Debug("New SSH channel",
			"type", newChannel.ChannelType(),
			"user", sshConn.User())

		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			log.Warn("Rejected unknown channel type", "type", newChannel.ChannelType())
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Error("Failed to accept channel", "error", err)
			continue
		}

		// Handle session
		go handleSession(channel, requests, sshConn, cfg, cpClient, log)
	}
}

func handleSession(channel ssh.Channel, requests <-chan *ssh.Request, sshConn *ssh.ServerConn, cfg *config.Config, cpClient *controlplane.Client, log *logger.Logger) {
	defer channel.Close()

	for req := range requests {
		log.Debug("SSH request", "type", req.Type, "want_reply", req.WantReply)

		switch req.Type {
		case "shell":
			// For now, just send a message and close
			req.Reply(true, nil)
			fmt.Fprintf(channel, "ZTNA Gateway v%s\r\n", version)
			fmt.Fprintf(channel, "User: %s\r\n", sshConn.User())
			fmt.Fprintf(channel, "TODO: SSH proxying not yet implemented\r\n")
			fmt.Fprintf(channel, "\r\nConnection will close.\r\n")
			channel.Close()
			return

		case "exec":
			// Command execution - will be used for proxying
			req.Reply(true, nil)
			fmt.Fprintf(channel, "Command execution not yet implemented\r\n")
			channel.CloseWrite()
			return

		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}
