#!/usr/bin/env python3
"""
ZTNA TCP Tunnel — Port-forward local via mTLS Gateway

Crée un listener TCP local et tunnelise chaque connexion entrante
à travers le gateway ZTNA via mTLS (TLS 1.3 + certificat client).

Usage:
    python3 scripts/ztna-tcp-tunnel.py \\
        --listen 15432 \\
        --gateway 10.10.10.20:4433 \\
        --cert /path/to/device.crt \\
        --key /path/to/device.key \\
        --resource "db:pg-staging"

    # Puis connecter psql :
    PGPASSWORD=ztna2026 psql -h localhost -p 15432 -U alice appdb
"""

import argparse
import json
import os
import select
import signal
import socket
import ssl
import sys
import threading

# ── Configuration ────────────────────────────────────────────────────────────

DEFAULT_GW_HOST = "10.10.10.20"
DEFAULT_GW_PORT = 4433
BUFFER_SIZE = 8192
CONNECT_TIMEOUT = 15


def log(msg: str) -> None:
    print(f"[tunnel] {msg}", flush=True)


def log_err(msg: str) -> None:
    print(f"[tunnel] ERROR: {msg}", file=sys.stderr, flush=True)


# ── mTLS Tunnel ──────────────────────────────────────────────────────────────

def create_mtls_tunnel(
    gw_host: str,
    gw_port: int,
    cert_file: str,
    key_file: str,
    resource_type: str,
    resource_match: str,
) -> tuple:
    """
    Établit une connexion mTLS vers le gateway, envoie le ConnectRequest,
    et retourne (tls_socket, remaining_data) si autorisé.
    """
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    ctx.load_cert_chain(certfile=cert_file, keyfile=key_file)

    raw = socket.create_connection((gw_host, gw_port), timeout=CONNECT_TIMEOUT)
    tls = ctx.wrap_socket(raw, server_hostname=gw_host)

    # Envoyer ConnectRequest (JSON + newline, protocole ZTNA gateway)
    connect_req = json.dumps({
        "resource_type": resource_type,
        "resource_match": resource_match,
        "action": "connect",
    })
    tls.sendall((connect_req + "\n").encode())

    # Lire ConnectResponse (JSON terminé par newline)
    buf = b""
    while b"\n" not in buf:
        chunk = tls.recv(BUFFER_SIZE)
        if not chunk:
            raise ConnectionError("Gateway a fermé la connexion avant la réponse")
        buf += chunk

    # Séparer la réponse JSON des éventuelles données restantes
    json_line, _, remaining = buf.partition(b"\n")
    resp = json.loads(json_line)

    if not resp.get("allowed"):
        reason = resp.get("reason", "inconnu")
        decision_id = resp.get("decision_id", "—")
        raise PermissionError(
            f"Accès refusé par le PEP — raison: {reason}, decision_id: {decision_id}"
        )

    decision_id = resp.get("decision_id", "—")
    log(f"AUTORISÉ (decision_id: {decision_id})")

    return tls, remaining


# ── TCP Proxy ────────────────────────────────────────────────────────────────

def proxy_bidirectional(client_sock: socket.socket, tunnel_sock, label: str) -> None:
    """Proxy TCP bidirectionnel entre le client local et le tunnel mTLS."""
    sockets = [client_sock, tunnel_sock]
    try:
        while True:
            readable, _, errored = select.select(sockets, [], sockets, 30.0)
            if errored:
                break
            for s in readable:
                data = s.recv(BUFFER_SIZE)
                if not data:
                    return  # Connexion fermée
                if s is client_sock:
                    tunnel_sock.sendall(data)
                else:
                    client_sock.sendall(data)
    except (ConnectionError, BrokenPipeError, OSError):
        pass
    finally:
        try:
            client_sock.close()
        except OSError:
            pass
        try:
            tunnel_sock.close()
        except OSError:
            pass
        log(f"[{label}] Connexion fermée")


# ── Port Forward Server ──────────────────────────────────────────────────────

def run_tunnel_server(
    listen_port: int,
    gw_host: str,
    gw_port: int,
    cert_file: str,
    key_file: str,
    resource_type: str,
    resource_match: str,
    single_shot: bool = False,
) -> None:
    """
    Écoute sur localhost:listen_port et tunnelise chaque connexion
    via mTLS vers le gateway ZTNA.
    """
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind(("127.0.0.1", listen_port))
    server.listen(5)

    log(f"Écoute sur 127.0.0.1:{listen_port}")
    log(f"Tunnel vers {gw_host}:{gw_port} → {resource_match}")

    # Signal de prêt (pour le script de démo)
    ready_file = os.environ.get("TUNNEL_READY_FILE")
    if ready_file:
        with open(ready_file, "w") as f:
            f.write(str(os.getpid()))

    conn_count = 0
    while True:
        try:
            client, addr = server.accept()
            conn_count += 1
            label = f"conn-{conn_count}"
            log(f"[{label}] Connexion locale depuis {addr[0]}:{addr[1]}")

            try:
                tls, remaining = create_mtls_tunnel(
                    gw_host, gw_port, cert_file, key_file,
                    resource_type, resource_match,
                )
            except PermissionError as e:
                log_err(f"[{label}] {e}")
                client.close()
                continue
            except Exception as e:
                log_err(f"[{label}] Erreur tunnel: {e}")
                client.close()
                continue

            # Envoyer les données restantes du buffer initial au client local
            if remaining:
                client.sendall(remaining)

            log(f"[{label}] Tunnel actif — proxy TCP bidirectionnel")

            if single_shot:
                # Mode single-shot : proxy dans le thread principal
                proxy_bidirectional(client, tls, label)
                break
            else:
                # Mode multi-connexion : proxy dans un thread séparé
                t = threading.Thread(
                    target=proxy_bidirectional,
                    args=(client, tls, label),
                    daemon=True,
                )
                t.start()

        except KeyboardInterrupt:
            log("Arrêt demandé (Ctrl+C)")
            break

    server.close()
    log("Tunnel fermé")


# ── Main ─────────────────────────────────────────────────────────────────────

def main() -> None:
    parser = argparse.ArgumentParser(
        description="ZTNA TCP Tunnel — Port-forward local via mTLS Gateway"
    )
    parser.add_argument(
        "--listen", "-l", type=int, required=True,
        help="Port local d'écoute (ex: 15432)"
    )
    parser.add_argument(
        "--gateway", "-g", type=str,
        default=f"{DEFAULT_GW_HOST}:{DEFAULT_GW_PORT}",
        help=f"Adresse du gateway host:port (défaut: {DEFAULT_GW_HOST}:{DEFAULT_GW_PORT})"
    )
    parser.add_argument(
        "--cert", "-c", type=str, required=True,
        help="Chemin du certificat device X.509 (PEM)"
    )
    parser.add_argument(
        "--key", "-k", type=str, required=True,
        help="Chemin de la clé privée device (PEM)"
    )
    parser.add_argument(
        "--resource", "-r", type=str, required=True,
        help='Resource match string (ex: "db:pg-staging")'
    )
    parser.add_argument(
        "--resource-type", "-t", type=str, default=None,
        help="Type de ressource (défaut: déduit du resource match)"
    )
    parser.add_argument(
        "--single-shot", action="store_true",
        help="Mode single-shot : accepte une seule connexion puis quitte"
    )

    args = parser.parse_args()

    # Parse gateway address
    if ":" in args.gateway:
        gw_host, gw_port_str = args.gateway.rsplit(":", 1)
        gw_port = int(gw_port_str)
    else:
        gw_host = args.gateway
        gw_port = DEFAULT_GW_PORT

    # Déduire le resource_type si pas spécifié
    resource_type = args.resource_type
    if not resource_type:
        resource_type = args.resource.split(":")[0] if ":" in args.resource else "tcp"

    # Vérifier les fichiers
    if not os.path.isfile(args.cert):
        log_err(f"Certificat non trouvé: {args.cert}")
        sys.exit(1)
    if not os.path.isfile(args.key):
        log_err(f"Clé non trouvée: {args.key}")
        sys.exit(1)

    # Gérer SIGTERM proprement
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))

    run_tunnel_server(
        listen_port=args.listen,
        gw_host=gw_host,
        gw_port=gw_port,
        cert_file=args.cert,
        key_file=args.key,
        resource_type=resource_type,
        resource_match=args.resource,
        single_shot=args.single_shot,
    )


if __name__ == "__main__":
    main()
