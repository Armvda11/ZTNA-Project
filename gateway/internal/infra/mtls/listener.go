// Package mtls gère le listener TLS de la Gateway avec authentification
// mutuelle (mTLS). Il configure le serveur TLS pour exiger et vérifier
// les certificats clients émis par le Control Plane.
//
// Politique de sécurité :
//   - TLS 1.3 minimum (pas de négociation descendante)
//   - ClientAuth = RequireAndVerifyClientCert (mTLS obligatoire)
//   - La CA client (client_ca_file) est celle utilisée par le CP pour signer
//     les certificats mTLS des clients ZTNA
//   - Les certificats clients sont de courte durée (15 min) ; le listener
//     vérifie automatiquement leur validité temporelle via la stack TLS
package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"ztna-gateway/internal/config"
	tlsutil "ztna-gateway/internal/infra/tls"
)

// ConnectionHandler est l'interface que doit implémenter le handler de
// connexion (typiquement protocol.Handler).
type ConnectionHandler interface {
	// HandleConnection traite une connexion mTLS entrante.
	// Le certificat client a déjà été vérifié par le listener TLS.
	HandleConnection(conn net.Conn, clientCert *x509.Certificate)
}

// Listener est le listener mTLS de la Gateway.
type Listener struct {
	cfg     *config.Config
	handler ConnectionHandler
	log     *slog.Logger
	ln      net.Listener
}

// NewListener crée un nouveau listener mTLS configuré mais pas encore
// en écoute.
func NewListener(cfg *config.Config, handler ConnectionHandler, log *slog.Logger) (*Listener, error) {
	return &Listener{
		cfg:     cfg,
		handler: handler,
		log:     log,
	}, nil
}

// buildTLSConfig construit la configuration TLS du serveur Gateway.
//
// Configuration de sécurité :
//   - MinVersion: TLS 1.3
//   - ClientAuth: RequireAndVerifyClientCert (mTLS obligatoire)
//   - ClientCAs: chargé depuis client_ca_file
//   - Certificat serveur: chargé depuis cert_file + key_file
func (l *Listener) buildTLSConfig() (*tls.Config, error) {
	// Charger le certificat serveur de la Gateway
	cert, err := tlsutil.LoadKeyPair(l.cfg.Server.TLS.CertFile, l.cfg.Server.TLS.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("impossible de charger le certificat serveur: %w", err)
	}

	// Charger la CA client pour vérifier les certificats des clients ZTNA
	clientCAs, err := tlsutil.LoadCertPoolFromPEMFile(l.cfg.Server.TLS.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("impossible de charger la CA client %s: %w", l.cfg.Server.TLS.ClientCAFile, err)
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
	}

	return tlsConfig, nil
}

// Listen démarre l'écoute mTLS sur l'adresse configurée et accepte les
// connexions entrantes jusqu'à l'annulation du contexte.
func (l *Listener) Listen(ctx context.Context) error {
	tlsConfig, err := l.buildTLSConfig()
	if err != nil {
		return fmt.Errorf("erreur de configuration TLS: %w", err)
	}

	ln, err := tls.Listen("tcp", l.cfg.Server.ListenAddr, tlsConfig)
	if err != nil {
		return fmt.Errorf("impossible de démarrer le listener TLS sur %s: %w", l.cfg.Server.ListenAddr, err)
	}
	l.ln = ln
	l.log.Info("listener mTLS démarré", "addr", l.cfg.Server.ListenAddr)

	// Fermer le listener dès que le contexte est annulé
	go func() {
		<-ctx.Done()
		l.log.Info("contexte annulé, fermeture du listener mTLS")
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Si le listener est fermé (arrêt normal), on sort proprement
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			l.log.Warn("erreur lors de l'acceptation d'une connexion", "error", err)
			continue
		}

		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			l.log.Error("connexion acceptée n'est pas TLS, fermée")
			conn.Close()
			continue
		}

		// Forcer le handshake TLS pour accéder au certificat client immédiatement
		if err := tlsConn.Handshake(); err != nil {
			l.log.Warn("échec du handshake TLS mTLS",
				"remote", conn.RemoteAddr().String(),
				"error", err,
			)
			conn.Close()
			continue
		}

		state := tlsConn.ConnectionState()
		if len(state.PeerCertificates) == 0 {
			l.log.Warn("aucun certificat client dans la connexion TLS, fermée",
				"remote", conn.RemoteAddr().String(),
			)
			conn.Close()
			continue
		}

		clientCert := state.PeerCertificates[0]
		l.log.Debug("connexion mTLS acceptée",
			"remote", conn.RemoteAddr().String(),
			"client_cn", clientCert.Subject.CommonName,
		)

		// Traiter chaque connexion dans une goroutine séparée
		go l.handler.HandleConnection(conn, clientCert)
	}
}

// Close ferme le listener mTLS.
func (l *Listener) Close() error {
	if l.ln != nil {
		return l.ln.Close()
	}
	return nil
}
