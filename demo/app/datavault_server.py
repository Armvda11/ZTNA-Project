#!/usr/bin/env python3
"""
DataVault — Serveur de ressource protégée pour la démo ZTNA
============================================================
Simule une API d'entreprise interne accessible uniquement via le tunnel ZTNA.

Usage :
    python3 datavault_server.py [--port 80] [--host 0.0.0.0]

Endpoints :
    GET /api/status          → santé du serveur (utilisé par le worker ZTNA)
    GET /api/vault/records   → liste des enregistrements sensibles (CONFIDENTIEL)
    GET /api/vault/secrets   → secrets critiques (TOP SECRET)
    GET /api/whoami          → identité de la connexion entrante (IP source)
    GET /                    → page d'accueil HTML
"""

import http.server
import json
import socket
import time
import argparse
import datetime
import os
import sys
import threading

START_TIME = time.time()
REQUEST_COUNT = 0
REQUEST_LOCK = threading.Lock()

VAULT_RECORDS = [
    {
        "id": "REC-001",
        "classification": "CONFIDENTIEL",
        "title": "Rapport Q4 2025 — Revenue ACME Corp",
        "owner": "finance@acme.corp",
        "created": "2025-01-15",
        "hash": "sha256:a3f8b2c1d4e7f9a0b1c2d3e4f5a6b7c8",
    },
    {
        "id": "REC-002",
        "classification": "CONFIDENTIEL",
        "title": "Liste clients stratégiques EMEA",
        "owner": "sales@acme.corp",
        "created": "2025-02-03",
        "hash": "sha256:1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e",
    },
    {
        "id": "REC-003",
        "classification": "INTERNE",
        "title": "Organigramme Direction Technique",
        "owner": "rh@acme.corp",
        "created": "2025-03-10",
        "hash": "sha256:9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c",
    },
    {
        "id": "REC-004",
        "classification": "CONFIDENTIEL",
        "title": "Budget Infrastructure Cloud 2026",
        "owner": "cto@acme.corp",
        "created": "2025-04-22",
        "hash": "sha256:3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f",
    },
]

VAULT_SECRETS = [
    {
        "id": "SEC-001",
        "classification": "🔐 TOP SECRET",
        "name": "prod-db-password",
        "description": "Mot de passe base de données production",
        "last_rotation": "2025-12-01",
        "expires": "2026-06-01",
        "value": "*** PROTÉGÉ PAR ZTNA — accès audité ***",
    },
    {
        "id": "SEC-002",
        "classification": "🔐 TOP SECRET",
        "name": "api-master-key",
        "description": "Clé maître API d'intégration partenaires",
        "last_rotation": "2025-11-15",
        "expires": "2026-05-15",
        "value": "*** PROTÉGÉ PAR ZTNA — accès audité ***",
    },
    {
        "id": "SEC-003",
        "classification": "CONFIDENTIEL",
        "name": "ldap-service-account",
        "description": "Compte de service LDAP / Active Directory",
        "last_rotation": "2025-10-01",
        "expires": "2026-04-01",
        "value": "*** PROTÉGÉ PAR ZTNA — accès audité ***",
    },
]


def json_response(handler, data, status=200):
    body = json.dumps(data, ensure_ascii=False, indent=2).encode("utf-8")
    handler.send_response(status)
    handler.send_header("Content-Type", "application/json; charset=utf-8")
    handler.send_header("Content-Length", str(len(body)))
    handler.send_header("X-DataVault-Host", socket.gethostname())
    handler.send_header("X-DataVault-Version", "1.0.0")
    handler.end_headers()
    handler.wfile.write(body)


def html_response(handler, html):
    body = html.encode("utf-8")
    handler.send_response(200)
    handler.send_header("Content-Type", "text/html; charset=utf-8")
    handler.send_header("Content-Length", str(len(body)))
    handler.end_headers()
    handler.wfile.write(body)


class DataVaultHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        now = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        src = self.client_address[0]
        print(f"[{now}] {src} — {fmt % args}", flush=True)

    def do_GET(self):
        global REQUEST_COUNT
        with REQUEST_LOCK:
            REQUEST_COUNT += 1
            req_num = REQUEST_COUNT

        path = self.path.split("?")[0].rstrip("/")
        source_ip = self.client_address[0]
        uptime = int(time.time() - START_TIME)
        now_iso = datetime.datetime.utcnow().isoformat() + "Z"
        hostname = socket.gethostname()

        # ── GET /api/status ────────────────────────────────────────────
        if path in ("", "/", "/api/status"):
            if path in ("", "/"):
                # Page d'accueil HTML
                html = f"""<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><title>DataVault — ACME Corp</title>
<style>body{{font-family:monospace;background:#0d1117;color:#c9d1d9;padding:2em}}
h1{{color:#58a6ff}}code{{background:#161b22;padding:2px 6px;border-radius:4px}}
table{{border-collapse:collapse;width:100%}}td,th{{border:1px solid #30363d;padding:8px}}
th{{background:#161b22}}</style></head>
<body>
<h1>🔐 DataVault API — ACME Corp Internal</h1>
<p>Accès via tunnel ZTNA mTLS uniquement. Ressource non routable depuis internet.</p>
<table>
<tr><th>Endpoint</th><th>Description</th></tr>
<tr><td><code>GET /api/status</code></td><td>Santé du serveur</td></tr>
<tr><td><code>GET /api/vault/records</code></td><td>Enregistrements confidentiels</td></tr>
<tr><td><code>GET /api/vault/secrets</code></td><td>Secrets critiques</td></tr>
<tr><td><code>GET /api/whoami</code></td><td>Identité de la connexion</td></tr>
</table>
<p>host: <strong>{hostname}</strong> | uptime: <strong>{uptime}s</strong> | requêtes: <strong>{req_num}</strong></p>
</body></html>"""
                html_response(self, html)
            else:
                json_response(self, {
                    "status": "ok",
                    "service": "datavault",
                    "version": "1.0.0",
                    "host": hostname,
                    "uptime_s": uptime,
                    "timestamp": now_iso,
                    "requests_served": req_num,
                    "classification": "INTERNE",
                    "message": "DataVault opérationnel — accès ZTNA autorisé",
                })
            return

        # ── GET /api/vault/records ─────────────────────────────────────
        if path == "/api/vault/records":
            json_response(self, {
                "classification": "🔒 CONFIDENTIEL",
                "records": VAULT_RECORDS,
                "count": len(VAULT_RECORDS),
                "host": hostname,
                "timestamp": now_iso,
                "accessed_from": source_ip,
                "notice": "Accès audité — ZTNA Zero Trust Network Access",
            })
            return

        # ── GET /api/vault/secrets ─────────────────────────────────────
        if path == "/api/vault/secrets":
            json_response(self, {
                "classification": "🔐 TOP SECRET",
                "secrets": VAULT_SECRETS,
                "count": len(VAULT_SECRETS),
                "host": hostname,
                "timestamp": now_iso,
                "accessed_from": source_ip,
                "notice": "ACCES AUDITÉ — Toute consultation est enregistrée",
                "warning": "Ces données ne doivent jamais quitter le périmètre ZTNA",
            })
            return

        # ── GET /api/whoami ────────────────────────────────────────────
        if path == "/api/whoami":
            json_response(self, {
                "source_ip": source_ip,
                "host": hostname,
                "timestamp": now_iso,
                "uptime_s": uptime,
                "note": "Connexion relayée via tunnel ZTNA mTLS — identité vérifiée par device-cert",
            })
            return

        # ── 404 ───────────────────────────────────────────────────────
        json_response(self, {
            "error": "not_found",
            "path": self.path,
            "available": ["/api/status", "/api/vault/records", "/api/vault/secrets", "/api/whoami"],
        }, status=404)


def main():
    parser = argparse.ArgumentParser(description="DataVault — Serveur de ressource protégée ZTNA")
    parser.add_argument("--host", default="0.0.0.0", help="Adresse d'écoute (défaut: 0.0.0.0)")
    parser.add_argument("--port", type=int, default=80, help="Port d'écoute (défaut: 80)")
    args = parser.parse_args()

    hostname = socket.gethostname()
    print(f"""
╔══════════════════════════════════════════════════════════╗
║          🔐 DataVault API — ACME Corp Internal           ║
╠══════════════════════════════════════════════════════════╣
║  Hôte    : {hostname:<44s} ║
║  Écoute  : {args.host}:{args.port:<40s} ║
║  Accès   : Tunnel ZTNA mTLS uniquement                   ║
╚══════════════════════════════════════════════════════════╝
""", flush=True)

    # Permettre la réutilisation du port (utile en dev)
    http.server.HTTPServer.allow_reuse_address = True
    server = http.server.HTTPServer((args.host, args.port), DataVaultHandler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n[DataVault] Arrêt du serveur.", flush=True)
        server.shutdown()


if __name__ == "__main__":
    main()
