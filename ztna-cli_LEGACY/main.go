package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	exitGeneric      = 1
	exitConfig       = 2
	exitAuth         = 3
	exitEnroll       = 4
	exitConnect      = 5
	exitPolicyDenied = 6
	exitRevoked      = 7
	exitExpired      = 8
	exitNetwork      = 9
)

const (
	defaultCPURL     = "https://10.10.20.30:8080"
	defaultGWAddr    = "10.10.10.20:4433"
	defaultIDPBase   = "http://10.10.20.30:8081"
	defaultRealm     = "ztna"
	defaultClientID  = "ztna-control-plane"
	defaultStateSub  = ".ztna"
	defaultTokenSkew = "2m"
	defaultCertSkew  = "24h"
)

type Config struct {
	Version  int           `json:"version"`
	Profile  string        `json:"profile"`
	StateDir string        `json:"state_dir"`
	CP       CPConfig      `json:"cp"`
	GW       GWConfig      `json:"gw"`
	IDP      IDPConfig     `json:"idp"`
	Runtime  RuntimeConfig `json:"runtime"`
	Logging  LoggingConfig `json:"logging"`
}

type CPConfig struct {
	URL         string `json:"url"`
	CACertPath  string `json:"ca_cert_path"`
	InsecureTLS bool   `json:"insecure_tls"`
	TimeoutSec  int    `json:"timeout_sec"`
}

type GWConfig struct {
	Addr        string `json:"addr"`
	ServerName  string `json:"server_name"`
	CACertPath  string `json:"ca_cert_path"`
	InsecureTLS bool   `json:"insecure_tls"`
	TimeoutSec  int    `json:"timeout_sec"`
}

type IDPConfig struct {
	BaseURL      string `json:"base_url"`
	Realm        string `json:"realm"`
	TokenURL     string `json:"token_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	InsecureTLS  bool   `json:"insecure_tls"`
	TimeoutSec   int    `json:"timeout_sec"`
}

type RuntimeConfig struct {
	TokenRenewBefore string `json:"token_renew_before"`
	CertRenewBefore  string `json:"cert_renew_before"`
	AutoRotateCert   bool   `json:"auto_rotate_cert"`
}

type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
	File   string `json:"file,omitempty"`
}

type State struct {
	Token  *TokenState  `json:"token,omitempty"`
	Device *DeviceState `json:"device,omitempty"`
}

type TokenState struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	Username     string    `json:"username,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DeviceState struct {
	KeyPath     string    `json:"key_path"`
	CertPath    string    `json:"cert_path"`
	CACertPath  string    `json:"ca_cert_path"`
	Serial      string    `json:"serial"`
	Fingerprint string    `json:"fingerprint"`
	ExpiresAt   time.Time `json:"expires_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type whoamiResponse struct {
	Sub      string   `json:"sub"`
	Username string   `json:"username"`
	Email    string   `json:"email,omitempty"`
	Groups   []string `json:"groups"`
}

type deviceCertRequest struct {
	CSRPEM     string `json:"csr_pem"`
	TTLSeconds *int64 `json:"ttl_seconds,omitempty"`
}

type deviceCertResponse struct {
	CertificatePEM string    `json:"certificate_pem"`
	CACertPEM      string    `json:"ca_cert_pem"`
	Serial         string    `json:"serial"`
	ExpiresAt      time.Time `json:"expires_at"`
	Fingerprint    string    `json:"fingerprint"`
}

type connectRequest struct {
	ResourceType  string `json:"resource_type"`
	ResourceMatch string `json:"resource_match"`
	Action        string `json:"action"`
}

type connectResponse struct {
	Allowed    bool   `json:"allowed"`
	DecisionID string `json:"decision_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

type revokeStatusOutput struct {
	Serial     string    `json:"serial"`
	Revoked    bool      `json:"revoked"`
	ThisUpdate time.Time `json:"this_update"`
	NextUpdate time.Time `json:"next_update"`
}

type cliError struct {
	code int
	msg  string
	err  error
}

func (e *cliError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.msg, e.err)
	}
	return e.msg
}

func (e *cliError) Unwrap() error { return e.err }
func (e *cliError) ExitCode() int { return e.code }

func fail(code int, msg string, err error) error {
	return &cliError{code: code, msg: msg, err: err}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(exitGeneric)
	}

	cmd := os.Args[1]
	args := os.Args[2:]
	var err error

	switch cmd {
	case "init":
		err = runInit(args)
	case "login":
		err = runLogin(args)
	case "enroll":
		err = runEnroll(args)
	case "connect":
		err = runConnect(args)
	case "whoami":
		err = runWhoami(args)
	case "status":
		err = runStatus(args)
	case "revoke-status":
		err = runRevokeStatus(args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		err = fail(exitGeneric, "unknown command", errors.New(cmd))
	}

	if err != nil {
		code := exitGeneric
		type exitCoder interface{ ExitCode() int }
		var ec exitCoder
		if errors.As(err, &ec) {
			code = ec.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(code)
	}
}

func usage() {
	fmt.Print(`ztna CLI

Usage:
  ztna init [flags]
  ztna login [flags]
  ztna enroll [flags]
  ztna connect <resource> [flags]
  ztna whoami [flags]
  ztna status [flags]
  ztna revoke-status [flags]

Commands:
  init          Configure CP/GW/IdP endpoints and local state path
  login         Obtain OIDC token from IdP (password or refresh)
  enroll        Create CSR and obtain a device X.509 certificate from CP
  connect       Open mTLS channel to gateway and relay traffic
  whoami        Show subject from CP /api/v1/whoami
  status        Show token/cert expiry + connectivity diagnostics
  revoke-status Check whether local device cert serial is revoked in CP CRL

Examples:
  ztna init --profile lab
  ztna login --username alice --password 'Password123!'
  ztna enroll
  ztna connect http:lan-app:80 --local-port 18080
  curl http://127.0.0.1:18080/
`)
}

func runInit(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fail(exitConfig, "resolve home directory", err)
	}
	defaultState := filepath.Join(home, defaultStateSub)
	defaultConfigPath := filepath.Join(defaultState, "config.json")

	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	stateDir := fs.String("state-dir", defaultState, "state directory")
	profile := fs.String("profile", "lab", "profile name")
	cpURL := fs.String("cp-url", defaultCPURL, "control-plane base URL")
	cpCA := fs.String("cp-ca", "", "control-plane CA cert path")
	cpInsecure := fs.Bool("cp-insecure", true, "skip CP TLS verification")
	gwAddr := fs.String("gw-addr", defaultGWAddr, "gateway address host:port")
	gwCA := fs.String("gw-ca", "", "gateway server CA cert path")
	gwInsecure := fs.Bool("gw-insecure", true, "skip gateway TLS verification")
	gwServerName := fs.String("gw-server-name", "", "gateway TLS server name")
	idpBase := fs.String("idp-base", defaultIDPBase, "IdP base URL")
	idpRealm := fs.String("idp-realm", defaultRealm, "IdP realm")
	idpTokenURL := fs.String("idp-token-url", "", "IdP token endpoint (optional)")
	idpClientID := fs.String("idp-client-id", defaultClientID, "OIDC client ID")
	idpClientSecret := fs.String("idp-client-secret", "", "OIDC client secret")
	idpInsecure := fs.Bool("idp-insecure", true, "skip IdP TLS verification")
	autoRotate := fs.Bool("auto-rotate-cert", true, "auto renew device cert when near expiry")
	tokenRenewBefore := fs.String("token-renew-before", defaultTokenSkew, "renew token before expiry")
	certRenewBefore := fs.String("cert-renew-before", defaultCertSkew, "renew cert before expiry")
	logLevel := fs.String("log-level", "info", "log level: debug|info|warn|error")
	logFormat := fs.String("log-format", "text", "log format: text|json")
	logFile := fs.String("log-file", "", "optional log file")
	fetchDeviceCA := fs.Bool("fetch-device-ca", false, "fetch /pki/device-ca/cert and store in state dir")

	if err := fs.Parse(args); err != nil {
		return fail(exitConfig, "parse init flags", err)
	}

	expStateDir, err := expandPath(*stateDir)
	if err != nil {
		return fail(exitConfig, "expand state dir", err)
	}
	expConfigPath, err := expandPath(*configPath)
	if err != nil {
		return fail(exitConfig, "expand config path", err)
	}
	if err := os.MkdirAll(expStateDir, 0o700); err != nil {
		return fail(exitConfig, "create state dir", err)
	}
	if err := os.MkdirAll(filepath.Dir(expConfigPath), 0o700); err != nil {
		return fail(exitConfig, "create config parent dir", err)
	}

	cfg := defaultConfig(expStateDir)
	cfg.Profile = *profile
	cfg.CP.URL = strings.TrimRight(*cpURL, "/")
	cfg.CP.CACertPath = *cpCA
	cfg.CP.InsecureTLS = *cpInsecure
	cfg.GW.Addr = *gwAddr
	cfg.GW.CACertPath = *gwCA
	cfg.GW.InsecureTLS = *gwInsecure
	cfg.GW.ServerName = *gwServerName
	cfg.IDP.BaseURL = strings.TrimRight(*idpBase, "/")
	cfg.IDP.Realm = *idpRealm
	cfg.IDP.ClientID = *idpClientID
	cfg.IDP.ClientSecret = *idpClientSecret
	cfg.IDP.InsecureTLS = *idpInsecure
	cfg.Runtime.AutoRotateCert = *autoRotate
	cfg.Runtime.TokenRenewBefore = *tokenRenewBefore
	cfg.Runtime.CertRenewBefore = *certRenewBefore
	cfg.Logging.Level = strings.ToLower(*logLevel)
	cfg.Logging.Format = strings.ToLower(*logFormat)
	cfg.Logging.File = *logFile
	if *idpTokenURL != "" {
		cfg.IDP.TokenURL = *idpTokenURL
	}

	if _, err := time.ParseDuration(cfg.Runtime.TokenRenewBefore); err != nil {
		return fail(exitConfig, "invalid token-renew-before", err)
	}
	if _, err := time.ParseDuration(cfg.Runtime.CertRenewBefore); err != nil {
		return fail(exitConfig, "invalid cert-renew-before", err)
	}

	if err := writeJSONFile(expConfigPath, cfg, 0o600); err != nil {
		return fail(exitConfig, "write config", err)
	}

	if *fetchDeviceCA {
		caPath := filepath.Join(expStateDir, "device-ca.crt")
		cli, err := newHTTPClient(cfg.CP.InsecureTLS, cfg.CP.CACertPath, timeoutDuration(cfg.CP.TimeoutSec, 10*time.Second))
		if err != nil {
			return fail(exitConfig, "build CP client", err)
		}
		resp, err := cli.Get(cfg.CP.URL + "/pki/device-ca/cert")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: unable to fetch device CA: %v\n", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				data, _ := io.ReadAll(resp.Body)
				if err := writeFile(caPath, data, 0o600); err != nil {
					fmt.Fprintf(os.Stderr, "warn: write device CA: %v\n", err)
				} else {
					fmt.Printf("device CA saved: %s\n", caPath)
				}
			} else {
				fmt.Fprintf(os.Stderr, "warn: unable to fetch device CA (HTTP %d)\n", resp.StatusCode)
			}
		}
	}

	fmt.Printf("config written: %s\n", expConfigPath)
	fmt.Printf("state dir: %s\n", expStateDir)
	return nil
}

func runLogin(args []string) error {
	home, _ := os.UserHomeDir()
	defaultConfigPath := filepath.Join(home, defaultStateSub, "config.json")

	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	username := fs.String("username", "", "OIDC username")
	password := fs.String("password", "", "OIDC password")
	passwordStdin := fs.Bool("password-stdin", false, "read password from stdin")
	verbose := fs.Bool("verbose", false, "enable debug logs")

	if err := fs.Parse(args); err != nil {
		return fail(exitConfig, "parse login flags", err)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	log, _, err := newLogger(cfg.Logging, *verbose)
	if err != nil {
		return fail(exitConfig, "configure logger", err)
	}

	state, err := loadState(cfg)
	if err != nil {
		return err
	}

	user := strings.TrimSpace(*username)
	if user == "" {
		if env := os.Getenv("ZTNA_USER"); env != "" {
			user = env
		} else if state.Token != nil {
			user = state.Token.Username
		}
	}
	if user == "" {
		return fail(exitAuth, "missing username", errors.New("provide --username or ZTNA_USER"))
	}

	pass := *password
	if *passwordStdin {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fail(exitAuth, "read password from stdin", err)
		}
		pass = strings.TrimSpace(string(b))
	}
	if pass == "" {
		pass = os.Getenv("ZTNA_PASS")
	}
	if pass == "" {
		return fail(exitAuth, "missing password", errors.New("provide --password, --password-stdin or ZTNA_PASS"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration(cfg.IDP.TimeoutSec, 10*time.Second))
	defer cancel()

	tok, err := requestPasswordToken(ctx, cfg, user, pass)
	if err != nil {
		return err
	}

	state.Token = tok
	if err := saveState(cfg, state); err != nil {
		return err
	}

	log.Info("login successful", slog.String("username", user), slog.Time("expires_at", tok.ExpiresAt))
	fmt.Printf("login ok: user=%s expires_at=%s\n", user, tok.ExpiresAt.Format(time.RFC3339))
	return nil
}

func runEnroll(args []string) error {
	home, _ := os.UserHomeDir()
	defaultConfigPath := filepath.Join(home, defaultStateSub, "config.json")

	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	ttlSeconds := fs.Int64("ttl-seconds", 0, "device cert TTL in seconds")
	forceKey := fs.Bool("force-new-key", false, "rotate device private key before enrollment")
	groupsRaw := fs.String("groups", "", "override groups in CSR (comma separated)")
	verbose := fs.Bool("verbose", false, "enable debug logs")

	if err := fs.Parse(args); err != nil {
		return fail(exitConfig, "parse enroll flags", err)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	log, _, err := newLogger(cfg.Logging, *verbose)
	if err != nil {
		return fail(exitConfig, "configure logger", err)
	}

	state, err := loadState(cfg)
	if err != nil {
		return err
	}
	ctx := context.Background()

	token, err := ensureAccessToken(ctx, cfg, state, log)
	if err != nil {
		return err
	}

	groups := parseCSV(*groupsRaw)
	ttl := int64(0)
	if *ttlSeconds > 0 {
		ttl = *ttlSeconds
	}

	device, subject, err := enrollDeviceCert(ctx, cfg, state, token, groups, ttl, *forceKey, log)
	if err != nil {
		return err
	}

	if err := saveState(cfg, state); err != nil {
		return err
	}

	fmt.Printf("enroll ok: user=%s serial=%s expires_at=%s\n", subject.Username, device.Serial, device.ExpiresAt.Format(time.RFC3339))
	return nil
}

func runWhoami(args []string) error {
	home, _ := os.UserHomeDir()
	defaultConfigPath := filepath.Join(home, defaultStateSub, "config.json")

	fs := flag.NewFlagSet("whoami", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	verbose := fs.Bool("verbose", false, "enable debug logs")
	if err := fs.Parse(args); err != nil {
		return fail(exitConfig, "parse whoami flags", err)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	log, _, err := newLogger(cfg.Logging, *verbose)
	if err != nil {
		return fail(exitConfig, "configure logger", err)
	}
	state, err := loadState(cfg)
	if err != nil {
		return err
	}

	ctx := context.Background()
	token, err := ensureAccessToken(ctx, cfg, state, log)
	if err != nil {
		return err
	}
	subject, err := fetchWhoami(ctx, cfg, token)
	if err != nil {
		return err
	}

	if state.Token != nil && state.Token.Username == "" {
		state.Token.Username = subject.Username
		_ = saveState(cfg, state)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(subject)
}

func runConnect(args []string) error {
	home, _ := os.UserHomeDir()
	defaultConfigPath := filepath.Join(home, defaultStateSub, "config.json")

	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	action := fs.String("action", "connect", "requested action")
	localPort := fs.Int("local-port", 0, "local TCP port for port-forward mode")
	listenHost := fs.String("listen-host", "127.0.0.1", "local listen host")
	httpProbe := fs.Bool("http-probe", false, "send one HTTP GET after authorization")
	httpPath := fs.String("http-path", "/", "HTTP path for --http-probe")
	noAutoRotate := fs.Bool("no-auto-rotate", false, "disable automatic cert rotation")
	verbose := fs.Bool("verbose", false, "enable debug logs")
	resourceArg, parsedArgs, err := splitConnectArgs(args)
	if err != nil {
		return fail(exitConnect, "parse connect args", err)
	}
	if err := fs.Parse(parsedArgs); err != nil {
		return fail(exitConfig, "parse connect flags", err)
	}
	if resourceArg == "" && fs.NArg() > 0 {
		resourceArg = fs.Arg(0)
	}
	if resourceArg == "" {
		return fail(exitConnect, "missing resource", errors.New("usage: ztna connect <resource>"))
	}
	if fs.NArg() > 1 {
		return fail(exitConnect, "unexpected extra arguments", errors.New(strings.Join(fs.Args()[1:], " ")))
	}

	resourceMatch := resourceArg
	resourceType, err := resourceTypeFromMatch(resourceMatch)
	if err != nil {
		return fail(exitConnect, "invalid resource", err)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	log, _, err := newLogger(cfg.Logging, *verbose)
	if err != nil {
		return fail(exitConfig, "configure logger", err)
	}
	state, err := loadState(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	autoRotate := cfg.Runtime.AutoRotateCert && !*noAutoRotate
	if err := ensureDeviceCert(ctx, cfg, state, autoRotate, log); err != nil {
		return err
	}
	if err := saveState(cfg, state); err != nil {
		return err
	}

	revoked, _, err := checkRevocationStatus(ctx, cfg, state)
	if err != nil {
		log.Warn("revocation check failed; continuing", slog.Any("err", err))
	} else if revoked {
		return fail(exitRevoked, "device certificate revoked", nil)
	}

	req := connectRequest{
		ResourceType:  resourceType,
		ResourceMatch: resourceMatch,
		Action:        *action,
	}

	if *localPort > 0 {
		return runPortForward(ctx, cfg, state, req, *listenHost, *localPort, log)
	}

	conn, resp, err := openAuthorizedGWConn(ctx, cfg, state, req)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Info("connect allowed", slog.String("decision_id", resp.DecisionID), slog.String("reason", resp.Reason))

	if *httpProbe {
		return runHTTPProbe(conn, resourceMatch, *httpPath)
	}
	return bridgeStdio(conn)
}

func runStatus(args []string) error {
	home, _ := os.UserHomeDir()
	defaultConfigPath := filepath.Join(home, defaultStateSub, "config.json")

	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	asJSON := fs.Bool("json", false, "output JSON")
	verbose := fs.Bool("verbose", false, "enable debug logs")
	if err := fs.Parse(args); err != nil {
		return fail(exitConfig, "parse status flags", err)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	log, _, err := newLogger(cfg.Logging, *verbose)
	if err != nil {
		return fail(exitConfig, "configure logger", err)
	}
	state, err := loadState(cfg)
	if err != nil {
		return err
	}

	tokenRenewBefore, _ := parseDuration(cfg.Runtime.TokenRenewBefore, 2*time.Minute)
	certRenewBefore, _ := parseDuration(cfg.Runtime.CertRenewBefore, 24*time.Hour)

	type tokenStatus struct {
		Present       bool       `json:"present"`
		Username      string     `json:"username,omitempty"`
		ExpiresAt     *time.Time `json:"expires_at,omitempty"`
		ExpiresIn     string     `json:"expires_in,omitempty"`
		NeedsRotation bool       `json:"needs_rotation"`
	}
	type deviceStatus struct {
		Present       bool       `json:"present"`
		Serial        string     `json:"serial,omitempty"`
		Fingerprint   string     `json:"fingerprint,omitempty"`
		ExpiresAt     *time.Time `json:"expires_at,omitempty"`
		ExpiresIn     string     `json:"expires_in,omitempty"`
		NeedsRotation bool       `json:"needs_rotation"`
	}
	type networkStatus struct {
		CPHealth    string `json:"cp_health"`
		GWReachable string `json:"gw_reachable"`
	}
	type out struct {
		Profile    string        `json:"profile"`
		ConfigPath string        `json:"config_path"`
		StateDir   string        `json:"state_dir"`
		Token      tokenStatus   `json:"token"`
		Device     deviceStatus  `json:"device"`
		Network    networkStatus `json:"network"`
	}
	status := out{
		Profile:    cfg.Profile,
		ConfigPath: *configPath,
		StateDir:   cfg.StateDir,
		Token:      tokenStatus{},
		Device:     deviceStatus{},
		Network:    networkStatus{CPHealth: "unknown", GWReachable: "unknown"},
	}

	now := time.Now()
	if state.Token != nil {
		status.Token.Present = true
		status.Token.Username = state.Token.Username
		ex := state.Token.ExpiresAt
		status.Token.ExpiresAt = &ex
		status.Token.ExpiresIn = time.Until(ex).Round(time.Second).String()
		status.Token.NeedsRotation = time.Until(ex) <= tokenRenewBefore
	}

	if state.Device != nil {
		status.Device.Present = true
		status.Device.Serial = state.Device.Serial
		status.Device.Fingerprint = state.Device.Fingerprint
		ex := state.Device.ExpiresAt
		status.Device.ExpiresAt = &ex
		status.Device.ExpiresIn = time.Until(ex).Round(time.Second).String()
		status.Device.NeedsRotation = time.Until(ex) <= certRenewBefore
	}

	cpClient, err := newHTTPClient(cfg.CP.InsecureTLS, cfg.CP.CACertPath, 4*time.Second)
	if err == nil {
		resp, err := cpClient.Get(cfg.CP.URL + "/healthz")
		if err != nil {
			status.Network.CPHealth = "down: " + err.Error()
		} else {
			_ = resp.Body.Close()
			status.Network.CPHealth = fmt.Sprintf("http_%d", resp.StatusCode)
		}
	} else {
		status.Network.CPHealth = "client_error: " + err.Error()
	}

	tcpConn, err := net.DialTimeout("tcp", cfg.GW.Addr, 4*time.Second)
	if err != nil {
		status.Network.GWReachable = "down: " + err.Error()
	} else {
		status.Network.GWReachable = "up"
		_ = tcpConn.Close()
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("profile: %s\n", status.Profile)
	fmt.Printf("config:  %s\n", status.ConfigPath)
	fmt.Printf("state:   %s\n", status.StateDir)
	fmt.Printf("cp:      %s\n", status.Network.CPHealth)
	fmt.Printf("gw:      %s\n", status.Network.GWReachable)
	if status.Token.Present {
		fmt.Printf("token:   user=%s expires_in=%s rotate=%v\n", status.Token.Username, status.Token.ExpiresIn, status.Token.NeedsRotation)
	} else {
		fmt.Printf("token:   missing\n")
	}
	if status.Device.Present {
		fmt.Printf("device:  serial=%s expires_in=%s rotate=%v\n", status.Device.Serial, status.Device.ExpiresIn, status.Device.NeedsRotation)
	} else {
		fmt.Printf("device:  missing\n")
	}
	log.Debug("status complete", slog.Time("at", now))
	return nil
}

func runRevokeStatus(args []string) error {
	home, _ := os.UserHomeDir()
	defaultConfigPath := filepath.Join(home, defaultStateSub, "config.json")

	fs := flag.NewFlagSet("revoke-status", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	verbose := fs.Bool("verbose", false, "enable debug logs")
	if err := fs.Parse(args); err != nil {
		return fail(exitConfig, "parse revoke-status flags", err)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	log, _, err := newLogger(cfg.Logging, *verbose)
	if err != nil {
		return fail(exitConfig, "configure logger", err)
	}
	state, err := loadState(cfg)
	if err != nil {
		return err
	}

	revoked, details, err := checkRevocationStatus(context.Background(), cfg, state)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(details); err != nil {
		return fail(exitGeneric, "encode output", err)
	}
	if revoked {
		log.Warn("certificate is revoked", slog.String("serial", details.Serial))
		return fail(exitRevoked, "certificate revoked", nil)
	}
	return nil
}

func defaultConfig(stateDir string) Config {
	return Config{
		Version:  1,
		Profile:  "lab",
		StateDir: stateDir,
		CP: CPConfig{
			URL:         defaultCPURL,
			InsecureTLS: true,
			TimeoutSec:  10,
		},
		GW: GWConfig{
			Addr:        defaultGWAddr,
			InsecureTLS: true,
			TimeoutSec:  10,
		},
		IDP: IDPConfig{
			BaseURL:     defaultIDPBase,
			Realm:       defaultRealm,
			ClientID:    defaultClientID,
			InsecureTLS: true,
			TimeoutSec:  10,
		},
		Runtime: RuntimeConfig{
			TokenRenewBefore: defaultTokenSkew,
			CertRenewBefore:  defaultCertSkew,
			AutoRotateCert:   true,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func loadConfig(path string) (Config, error) {
	expanded, err := expandPath(path)
	if err != nil {
		return Config{}, fail(exitConfig, "expand config path", err)
	}
	data, err := os.ReadFile(expanded)
	if err != nil {
		return Config{}, fail(exitConfig, "read config", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fail(exitConfig, "parse config", err)
	}
	if cfg.StateDir == "" {
		cfg.StateDir = filepath.Join(filepath.Dir(expanded), "")
	}
	cfg.StateDir, err = expandPath(cfg.StateDir)
	if err != nil {
		return Config{}, fail(exitConfig, "expand state dir", err)
	}
	if cfg.CP.URL == "" || cfg.GW.Addr == "" {
		return Config{}, fail(exitConfig, "invalid config", errors.New("cp.url and gw.addr are required"))
	}
	if cfg.IDP.TokenURL == "" {
		cfg.IDP.TokenURL = deriveTokenURL(cfg.IDP.BaseURL, cfg.IDP.Realm)
	}
	if cfg.IDP.ClientID == "" {
		cfg.IDP.ClientID = defaultClientID
	}
	return cfg, nil
}

func statePath(cfg Config) string {
	return filepath.Join(cfg.StateDir, "state.json")
}

func loadState(cfg Config) (*State, error) {
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, fail(exitConfig, "create state dir", err)
	}
	path := statePath(cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{}, nil
		}
		return nil, fail(exitConfig, "read state", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fail(exitConfig, "parse state", err)
	}
	if st.Device != nil {
		if st.Device.KeyPath == "" {
			st.Device.KeyPath = filepath.Join(cfg.StateDir, "device.key")
		}
		if st.Device.CertPath == "" {
			st.Device.CertPath = filepath.Join(cfg.StateDir, "device.crt")
		}
		if st.Device.CACertPath == "" {
			st.Device.CACertPath = filepath.Join(cfg.StateDir, "device-ca.crt")
		}
	}
	return &st, nil
}

func saveState(cfg Config, st *State) error {
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return fail(exitConfig, "create state dir", err)
	}
	if err := writeJSONFile(statePath(cfg), st, 0o600); err != nil {
		return fail(exitConfig, "write state", err)
	}
	return nil
}

func requestPasswordToken(ctx context.Context, cfg Config, username, password string) (*TokenState, error) {
	values := url.Values{}
	values.Set("grant_type", "password")
	values.Set("client_id", cfg.IDP.ClientID)
	if cfg.IDP.ClientSecret != "" {
		values.Set("client_secret", cfg.IDP.ClientSecret)
	}
	values.Set("username", username)
	values.Set("password", password)

	resp, err := requestOIDCToken(ctx, cfg, values)
	if err != nil {
		return nil, err
	}
	return buildTokenState(resp, username)
}

func refreshAccessToken(ctx context.Context, cfg Config, refreshToken, username string) (*TokenState, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("client_id", cfg.IDP.ClientID)
	if cfg.IDP.ClientSecret != "" {
		values.Set("client_secret", cfg.IDP.ClientSecret)
	}
	values.Set("refresh_token", refreshToken)

	resp, err := requestOIDCToken(ctx, cfg, values)
	if err != nil {
		return nil, err
	}
	if resp.RefreshToken == "" {
		resp.RefreshToken = refreshToken
	}
	return buildTokenState(resp, username)
}

func requestOIDCToken(ctx context.Context, cfg Config, values url.Values) (tokenResponse, error) {
	client, err := newHTTPClient(cfg.IDP.InsecureTLS, "", timeoutDuration(cfg.IDP.TimeoutSec, 10*time.Second))
	if err != nil {
		return tokenResponse{}, fail(exitAuth, "build IdP client", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.IDP.TokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse{}, fail(exitAuth, "build token request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, fail(exitNetwork, "IdP token request failed", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var token tokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return tokenResponse{}, fail(exitAuth, "decode token response", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := token.ErrorDescription
		if msg == "" {
			msg = token.Error
		}
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		return tokenResponse{}, fail(exitAuth, "OIDC login failed", errors.New(msg))
	}

	if token.AccessToken == "" {
		return tokenResponse{}, fail(exitAuth, "OIDC token missing access_token", nil)
	}
	return token, nil
}

func buildTokenState(resp tokenResponse, username string) (*TokenState, error) {
	expiresAt := time.Time{}
	if resp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	} else {
		decoded, err := tokenExpiry(resp.AccessToken)
		if err != nil {
			return nil, fail(exitAuth, "resolve token expiration", err)
		}
		expiresAt = decoded
	}
	return &TokenState{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresAt:    expiresAt,
		Username:     username,
		UpdatedAt:    time.Now(),
	}, nil
}

func tokenExpiry(jwt string) (time.Time, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return time.Time{}, errors.New("invalid jwt format")
	}
	payload, err := decodeBase64URL(parts[1])
	if err != nil {
		return time.Time{}, err
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, err
	}
	rawExp, ok := claims["exp"]
	if !ok {
		return time.Time{}, errors.New("jwt exp claim not found")
	}
	expFloat, ok := rawExp.(float64)
	if !ok {
		return time.Time{}, errors.New("jwt exp claim not numeric")
	}
	return time.Unix(int64(expFloat), 0), nil
}

func decodeBase64URL(s string) ([]byte, error) {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.URLEncoding.DecodeString(s)
}

func ensureAccessToken(ctx context.Context, cfg Config, st *State, log *slog.Logger) (string, error) {
	if st.Token == nil || st.Token.AccessToken == "" {
		return "", fail(exitAuth, "no access token", errors.New("run: ztna login"))
	}

	renewBefore, err := parseDuration(cfg.Runtime.TokenRenewBefore, 2*time.Minute)
	if err != nil {
		return "", fail(exitConfig, "invalid runtime.token_renew_before", err)
	}

	if st.Token.ExpiresAt.IsZero() {
		if exp, err := tokenExpiry(st.Token.AccessToken); err == nil {
			st.Token.ExpiresAt = exp
		}
	}
	if time.Until(st.Token.ExpiresAt) > renewBefore {
		return st.Token.AccessToken, nil
	}

	if st.Token.RefreshToken == "" {
		return "", fail(exitExpired, "access token expired", errors.New("refresh token not available; run: ztna login"))
	}

	log.Info("refreshing OIDC token", slog.Time("expires_at", st.Token.ExpiresAt))
	ctx, cancel := context.WithTimeout(ctx, timeoutDuration(cfg.IDP.TimeoutSec, 10*time.Second))
	defer cancel()

	newToken, err := refreshAccessToken(ctx, cfg, st.Token.RefreshToken, st.Token.Username)
	if err != nil {
		return "", fail(exitAuth, "refresh token failed", err)
	}
	st.Token = newToken
	if err := saveState(cfg, st); err != nil {
		return "", err
	}

	return st.Token.AccessToken, nil
}

func fetchWhoami(ctx context.Context, cfg Config, accessToken string) (whoamiResponse, error) {
	cli, err := newHTTPClient(cfg.CP.InsecureTLS, cfg.CP.CACertPath, timeoutDuration(cfg.CP.TimeoutSec, 10*time.Second))
	if err != nil {
		return whoamiResponse{}, fail(exitConfig, "build CP client", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.CP.URL+"/api/v1/whoami", nil)
	if err != nil {
		return whoamiResponse{}, fail(exitNetwork, "build whoami request", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := cli.Do(req)
	if err != nil {
		return whoamiResponse{}, fail(exitNetwork, "whoami request failed", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return whoamiResponse{}, fail(exitAuth, "whoami failed", decodeAPIError(resp.StatusCode, body))
	}

	var subject whoamiResponse
	if err := json.Unmarshal(body, &subject); err != nil {
		return whoamiResponse{}, fail(exitGeneric, "decode whoami response", err)
	}
	if subject.Username == "" {
		subject.Username = subject.Sub
	}
	return subject, nil
}

func enrollDeviceCert(
	ctx context.Context,
	cfg Config,
	st *State,
	token string,
	groupOverride []string,
	ttlSeconds int64,
	forceNewKey bool,
	log *slog.Logger,
) (*DeviceState, whoamiResponse, error) {
	subject, err := fetchWhoami(ctx, cfg, token)
	if err != nil {
		return nil, whoamiResponse{}, err
	}
	groups := subject.Groups
	if len(groupOverride) > 0 {
		groups = groupOverride
	}

	keyPath, certPath, caPath := devicePaths(cfg, st)
	signer, err := ensureDeviceKey(keyPath, forceNewKey)
	if err != nil {
		return nil, subject, fail(exitEnroll, "prepare device key", err)
	}

	csrPEM, err := buildCSRPEM(signer, subject.Username, groups)
	if err != nil {
		return nil, subject, fail(exitEnroll, "build CSR", err)
	}

	reqBody := deviceCertRequest{CSRPEM: string(csrPEM)}
	if ttlSeconds > 0 {
		reqBody.TTLSeconds = &ttlSeconds
	}

	cli, err := newHTTPClient(cfg.CP.InsecureTLS, cfg.CP.CACertPath, timeoutDuration(cfg.CP.TimeoutSec, 12*time.Second))
	if err != nil {
		return nil, subject, fail(exitConfig, "build CP client", err)
	}
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.CP.URL+"/api/v1/credentials/device-cert", bytes.NewReader(payload))
	if err != nil {
		return nil, subject, fail(exitNetwork, "build enroll request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cli.Do(req)
	if err != nil {
		return nil, subject, fail(exitNetwork, "device cert enrollment failed", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, subject, fail(exitEnroll, "device cert enrollment denied", decodeAPIError(resp.StatusCode, body))
	}

	var certResp deviceCertResponse
	if err := json.Unmarshal(body, &certResp); err != nil {
		return nil, subject, fail(exitEnroll, "decode enrollment response", err)
	}
	if certResp.CertificatePEM == "" {
		return nil, subject, fail(exitEnroll, "certificate_pem missing", nil)
	}
	if err := writeFile(certPath, []byte(certResp.CertificatePEM), 0o600); err != nil {
		return nil, subject, fail(exitEnroll, "write device certificate", err)
	}
	if certResp.CACertPEM != "" {
		if err := writeFile(caPath, []byte(certResp.CACertPEM), 0o600); err != nil {
			return nil, subject, fail(exitEnroll, "write device CA", err)
		}
	}

	expiresAt := certResp.ExpiresAt
	if expiresAt.IsZero() {
		if cert, err := loadPEMCert(certPath); err == nil {
			expiresAt = cert.NotAfter
		}
	}
	dev := &DeviceState{
		KeyPath:     keyPath,
		CertPath:    certPath,
		CACertPath:  caPath,
		Serial:      certResp.Serial,
		Fingerprint: certResp.Fingerprint,
		ExpiresAt:   expiresAt,
		UpdatedAt:   time.Now(),
	}
	st.Device = dev
	if st.Token != nil && st.Token.Username == "" {
		st.Token.Username = subject.Username
	}
	log.Info("device enrolled", slog.String("serial", dev.Serial), slog.Time("expires_at", dev.ExpiresAt))
	return dev, subject, nil
}

func ensureDeviceCert(ctx context.Context, cfg Config, st *State, autoRotate bool, log *slog.Logger) error {
	keyPath, certPath, caPath := devicePaths(cfg, st)
	if st.Device == nil {
		st.Device = &DeviceState{KeyPath: keyPath, CertPath: certPath, CACertPath: caPath}
	}

	cert, err := loadPEMCert(certPath)
	if err != nil {
		if autoRotate {
			log.Warn("device cert missing, triggering enroll", slog.Any("err", err))
			token, err := ensureAccessToken(ctx, cfg, st, log)
			if err != nil {
				return err
			}
			_, _, err = enrollDeviceCert(ctx, cfg, st, token, nil, 0, false, log)
			return err
		}
		return fail(exitExpired, "device certificate missing", errors.New("run: ztna enroll"))
	}

	if st.Device.Serial == "" {
		st.Device.Serial = strings.ToLower(cert.SerialNumber.Text(16))
	}
	if st.Device.ExpiresAt.IsZero() {
		st.Device.ExpiresAt = cert.NotAfter
	}

	if _, err := os.Stat(keyPath); err != nil {
		if autoRotate {
			log.Warn("device key missing, re-enrolling", slog.Any("err", err))
			token, err := ensureAccessToken(ctx, cfg, st, log)
			if err != nil {
				return err
			}
			_, _, err = enrollDeviceCert(ctx, cfg, st, token, nil, 0, true, log)
			return err
		}
		return fail(exitExpired, "device private key missing", errors.New("run: ztna enroll --force-new-key"))
	}

	reneBefore, err := parseDuration(cfg.Runtime.CertRenewBefore, 24*time.Hour)
	if err != nil {
		return fail(exitConfig, "invalid runtime.cert_renew_before", err)
	}

	until := time.Until(cert.NotAfter)
	if until <= 0 {
		if !autoRotate {
			return fail(exitExpired, "device certificate expired", errors.New("run: ztna enroll"))
		}
		log.Warn("device certificate expired, renewing", slog.Time("expired_at", cert.NotAfter))
		token, err := ensureAccessToken(ctx, cfg, st, log)
		if err != nil {
			return err
		}
		_, _, err = enrollDeviceCert(ctx, cfg, st, token, nil, 0, false, log)
		return err
	}

	if until <= reneBefore && autoRotate {
		log.Info("device certificate near expiry, auto-rotating", slog.Duration("expires_in", until))
		token, err := ensureAccessToken(ctx, cfg, st, log)
		if err != nil {
			return err
		}
		_, _, err = enrollDeviceCert(ctx, cfg, st, token, nil, 0, false, log)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkRevocationStatus(ctx context.Context, cfg Config, st *State) (bool, revokeStatusOutput, error) {
	if st.Device == nil {
		return false, revokeStatusOutput{}, fail(exitRevoked, "device state missing", errors.New("run: ztna enroll"))
	}
	cert, err := loadPEMCert(st.Device.CertPath)
	if err != nil {
		return false, revokeStatusOutput{}, fail(exitRevoked, "read local certificate", err)
	}

	cli, err := newHTTPClient(cfg.CP.InsecureTLS, cfg.CP.CACertPath, timeoutDuration(cfg.CP.TimeoutSec, 10*time.Second))
	if err != nil {
		return false, revokeStatusOutput{}, fail(exitConfig, "build CP client", err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.CP.URL+"/pki/device-ca/crl", nil)
	resp, err := cli.Do(req)
	if err != nil {
		return false, revokeStatusOutput{}, fail(exitNetwork, "fetch CRL", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false, revokeStatusOutput{}, fail(exitNetwork, "fetch CRL failed", decodeAPIError(resp.StatusCode, body))
	}

	crl, err := x509.ParseRevocationList(body)
	if err != nil {
		return false, revokeStatusOutput{}, fail(exitGeneric, "parse CRL", err)
	}

	revoked := false
	for _, entry := range crl.RevokedCertificateEntries {
		if entry.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			revoked = true
			break
		}
	}
	out := revokeStatusOutput{
		Serial:     strings.ToLower(cert.SerialNumber.Text(16)),
		Revoked:    revoked,
		ThisUpdate: crl.ThisUpdate,
		NextUpdate: crl.NextUpdate,
	}
	return revoked, out, nil
}

func openAuthorizedGWConn(ctx context.Context, cfg Config, st *State, req connectRequest) (net.Conn, connectResponse, error) {
	if st.Device == nil {
		return nil, connectResponse{}, fail(exitConnect, "device identity missing", errors.New("run: ztna enroll"))
	}
	certPair, err := tls.LoadX509KeyPair(st.Device.CertPath, st.Device.KeyPath)
	if err != nil {
		return nil, connectResponse{}, fail(exitConnect, "load device keypair", err)
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{certPair},
		InsecureSkipVerify: cfg.GW.InsecureTLS,
	}
	if !cfg.GW.InsecureTLS && cfg.GW.CACertPath != "" {
		pool, err := certPoolFromFile(cfg.GW.CACertPath)
		if err != nil {
			return nil, connectResponse{}, fail(exitConfig, "load gateway CA cert", err)
		}
		tlsCfg.RootCAs = pool
	}
	if cfg.GW.ServerName != "" {
		tlsCfg.ServerName = cfg.GW.ServerName
	}

	d := net.Dialer{Timeout: timeoutDuration(cfg.GW.TimeoutSec, 10*time.Second)}
	rawConn, err := d.DialContext(ctx, "tcp", cfg.GW.Addr)
	if err != nil {
		return nil, connectResponse{}, fail(exitNetwork, "dial gateway", err)
	}

	tlsConn := tls.Client(rawConn, tlsCfg)
	if err := tlsConn.SetDeadline(time.Now().Add(timeoutDuration(cfg.GW.TimeoutSec, 10*time.Second))); err == nil {
		defer tlsConn.SetDeadline(time.Time{})
	}
	if err := tlsConn.Handshake(); err != nil {
		_ = rawConn.Close()
		return nil, connectResponse{}, fail(exitConnect, "gateway TLS handshake failed", err)
	}

	enc := json.NewEncoder(tlsConn)
	if err := enc.Encode(req); err != nil {
		_ = rawConn.Close()
		return nil, connectResponse{}, fail(exitConnect, "send connect request", err)
	}

	reader := bufio.NewReader(tlsConn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		_ = rawConn.Close()
		return nil, connectResponse{}, fail(exitConnect, "read connect response", err)
	}
	var resp connectResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		_ = rawConn.Close()
		return nil, connectResponse{}, fail(exitConnect, "decode connect response", err)
	}
	if !resp.Allowed {
		_ = rawConn.Close()
		return nil, resp, fail(exitPolicyDenied, "gateway denied access", errors.New(resp.Reason))
	}

	return &bufferedConn{Conn: tlsConn, reader: reader}, resp, nil
}

func runPortForward(
	ctx context.Context,
	cfg Config,
	st *State,
	req connectRequest,
	listenHost string,
	localPort int,
	log *slog.Logger,
) error {
	addr := fmt.Sprintf("%s:%d", listenHost, localPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fail(exitConnect, "listen local port", err)
	}
	defer ln.Close()

	log.Info("port-forward ready",
		slog.String("listen", addr),
		slog.String("resource", req.ResourceMatch),
		slog.String("gateway", cfg.GW.Addr),
	)
	fmt.Printf("forwarding %s -> %s via %s\n", addr, req.ResourceMatch, cfg.GW.Addr)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		localConn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Warn("accept local connection failed", slog.Any("err", err))
			continue
		}

		go func(c net.Conn) {
			defer c.Close()
			gwConn, resp, err := openAuthorizedGWConn(ctx, cfg, st, req)
			if err != nil {
				log.Warn("gateway authorization failed", slog.Any("err", err))
				return
			}
			defer gwConn.Close()
			log.Info("forward connection allowed",
				slog.String("remote", c.RemoteAddr().String()),
				slog.String("decision_id", resp.DecisionID),
			)
			proxyBothWays(c, gwConn)
		}(localConn)
	}
}

func runHTTPProbe(conn net.Conn, resourceMatch, path string) error {
	host := strings.TrimPrefix(resourceMatch, "http:")
	if i := strings.Index(host, ":"); i > 0 {
		host = host[:i]
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", path, host)
	if _, err := conn.Write([]byte(req)); err != nil {
		return fail(exitConnect, "send HTTP probe", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(15 * time.Second)); err == nil {
		defer conn.SetReadDeadline(time.Time{})
	}
	_, err := io.Copy(os.Stdout, conn)
	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		return fail(exitConnect, "read HTTP response", err)
	}
	return nil
}

func bridgeStdio(conn net.Conn) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn, os.Stdin)
		closeWrite(conn)
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		_, err := io.Copy(os.Stdout, conn)
		errCh <- err
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return fail(exitConnect, "stream relay failed", err)
		}
	}
	return nil
}

func proxyBothWays(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		closeWrite(b)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		closeWrite(a)
	}()
	wg.Wait()
}

func closeWrite(conn net.Conn) {
	type closeWriter interface {
		CloseWrite() error
	}
	if cw, ok := conn.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}

func resourceTypeFromMatch(resource string) (string, error) {
	if strings.HasPrefix(resource, "http:") {
		return "http", nil
	}
	if strings.HasPrefix(resource, "ssh:") {
		return "ssh", nil
	}
	return "", errors.New("resource must start with http: or ssh:")
}

func devicePaths(cfg Config, st *State) (keyPath, certPath, caPath string) {
	keyPath = filepath.Join(cfg.StateDir, "device.key")
	certPath = filepath.Join(cfg.StateDir, "device.crt")
	caPath = filepath.Join(cfg.StateDir, "device-ca.crt")
	if st != nil && st.Device != nil {
		if st.Device.KeyPath != "" {
			keyPath = st.Device.KeyPath
		}
		if st.Device.CertPath != "" {
			certPath = st.Device.CertPath
		}
		if st.Device.CACertPath != "" {
			caPath = st.Device.CACertPath
		}
	}
	return
}

func ensureDeviceKey(path string, forceNew bool) (crypto.Signer, error) {
	if !forceNew {
		if signer, err := readSigner(path); err == nil {
			return signer, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := writeFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func readSigner(path string) (crypto.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(data)
	if blk == nil {
		return nil, errors.New("invalid PEM private key")
	}
	switch blk.Type {
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(blk.Bytes)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
		if err != nil {
			return nil, err
		}
		signer, ok := k.(crypto.Signer)
		if !ok {
			return nil, errors.New("unsupported private key type")
		}
		return signer, nil
	default:
		return nil, fmt.Errorf("unsupported private key type: %s", blk.Type)
	}
}

func buildCSRPEM(key crypto.Signer, username string, groups []string) ([]byte, error) {
	tpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   username,
			Organization: groups,
		},
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tpl, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

func loadPEMCert(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(data)
	if blk == nil {
		return nil, errors.New("invalid PEM certificate")
	}
	return x509.ParseCertificate(blk.Bytes)
}

func certPoolFromFile(path string) (*x509.CertPool, error) {
	exp, err := expandPath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(exp)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, errors.New("invalid PEM CA certificate")
	}
	return pool, nil
}

func newHTTPClient(insecure bool, caPath string, timeout time.Duration) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure}
	if !insecure && strings.TrimSpace(caPath) != "" {
		pool, err := certPoolFromFile(caPath)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	}
	transport := &http.Transport{TLSClientConfig: tlsCfg}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

func decodeAPIError(code int, body []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if msg, ok := payload["error"].(string); ok && msg != "" {
			return fmt.Errorf("HTTP %d: %s", code, msg)
		}
		if msg, ok := payload["message"].(string); ok && msg != "" {
			return fmt.Errorf("HTTP %d: %s", code, msg)
		}
	}
	trim := strings.TrimSpace(string(body))
	if trim == "" {
		trim = http.StatusText(code)
	}
	return fmt.Errorf("HTTP %d: %s", code, trim)
}

func writeJSONFile(path string, v any, perm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data, perm)
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	exp, err := expandPath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(exp), 0o700); err != nil {
		return err
	}
	tmp := exp + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, exp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func deriveTokenURL(baseURL, realm string) string {
	base := strings.TrimRight(baseURL, "/")
	realm = strings.TrimPrefix(strings.TrimSpace(realm), "/")
	if realm == "" {
		realm = defaultRealm
	}
	return fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", base, realm)
}

func parseDuration(raw string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	return time.ParseDuration(raw)
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func timeoutDuration(sec int, fallback time.Duration) time.Duration {
	if sec <= 0 {
		return fallback
	}
	return time.Duration(sec) * time.Second
}

func splitConnectArgs(args []string) (string, []string, error) {
	needsValue := map[string]bool{
		"--config":      true,
		"--action":      true,
		"--local-port":  true,
		"--listen-host": true,
		"--http-path":   true,
	}

	filtered := make([]string, 0, len(args))
	resource := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			name, _, hasEq := strings.Cut(arg, "=")
			filtered = append(filtered, arg)
			if needsValue[name] && !hasEq {
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("flag %s requires a value", name)
				}
				filtered = append(filtered, args[i+1])
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			filtered = append(filtered, arg)
			continue
		}
		if resource == "" {
			resource = arg
			continue
		}
		filtered = append(filtered, arg)
	}
	return resource, filtered, nil
}

func newLogger(cfg LoggingConfig, verbose bool) (*slog.Logger, io.Closer, error) {
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	if verbose {
		level.Set(slog.LevelDebug)
	} else {
		switch strings.ToLower(cfg.Level) {
		case "debug":
			level.Set(slog.LevelDebug)
		case "warn":
			level.Set(slog.LevelWarn)
		case "error":
			level.Set(slog.LevelError)
		default:
			level.Set(slog.LevelInfo)
		}
	}

	writer := io.Writer(os.Stderr)
	var closer io.Closer
	if cfg.File != "" {
		path, err := expandPath(cfg.File)
		if err != nil {
			return nil, nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, err
		}
		writer = io.MultiWriter(os.Stderr, f)
		closer = f
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "json") {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}
	return slog.New(handler), closer, nil
}

// bigFromHex keeps a dedicated converter for serial formatting compatibility.
func bigFromHex(s string) *big.Int {
	n := new(big.Int)
	n.SetString(s, 16)
	return n
}
