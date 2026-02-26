#!/usr/bin/env python3
"""
ztna — Zero Trust Network Access CLI
Outil client installé sur wan-client par l'étape 3 de la démo.

Usage:
  ztna whoami                   Identité du device (certificat X.509)
  ztna status                   État du gateway et de l'API DataVault
  ztna get <endpoint>           Accès à une ressource via tunnel ZTNA
  ztna vault records            Liste des enregistrements confidentiels
  ztna vault secrets            Secrets critiques TOP SECRET
  ztna vault whoami             Identité de la connexion côté serveur
"""
import ssl, socket, struct, json, sys, os, subprocess, re

# ─── Config (lue depuis /tmp/ztna-demo/) ──────────────────────────────────────
CONF_DIR    = "/tmp/ztna-demo"
GW_PORT     = 4433
CERT_FILE   = f"{CONF_DIR}/device.crt"
KEY_FILE    = f"{CONF_DIR}/device.key"
TARGET      = "lan-app"
TARGET_PORT = 80

def _gw_host():
    try:
        return open(f"{CONF_DIR}/gw_addr.txt").read().split(":")[0].strip()
    except Exception:
        return "10.10.10.20"

GW_HOST = _gw_host()

# ─── Couleurs ANSI ────────────────────────────────────────────────────────────
G  = "\033[0;32m"   # vert
R  = "\033[0;31m"   # rouge
C  = "\033[0;36m"   # cyan
Y  = "\033[0;33m"   # jaune
B  = "\033[1m"      # gras
D  = "\033[2m"      # dim
W  = "\033[0;37m"   # blanc
N  = "\033[0m"      # reset

# ─── Helpers ──────────────────────────────────────────────────────────────────
def die(msg):
    print(f"\n  {R}[error]{N} {msg}\n", file=sys.stderr)
    sys.exit(1)

def check_cert():
    if not os.path.exists(CERT_FILE):
        die(f"Certificat introuvable: {CERT_FILE}\n         Lancez d'abord l'étape 3 : make demo-manual")

def cert_info():
    try:
        out = subprocess.check_output(
            ["openssl","x509","-noout","-subject","-serial","-dates","-in",CERT_FILE],
            stderr=subprocess.DEVNULL).decode()
        d = {}
        for line in out.splitlines():
            k, _, v = line.partition("=")
            k = k.strip().lower()
            if   "subject"   in k:               d["subject"]   = v.strip()
            elif "serial"    in k:               d["serial"]    = v.strip().lower()
            elif "notbefore" in k.replace(" ",""): d["not_before"] = v.strip()
            elif "notafter"  in k.replace(" ",""): d["not_after"]  = v.strip()
        cn  = re.search(r"CN\s*=\s*([^,/]+)", d.get("subject",""))
        org = re.search(r"O\s*=\s*([^,/]+)",  d.get("subject",""))
        d["cn"]  = cn.group(1).strip()  if cn  else "?"
        d["org"] = org.group(1).strip() if org else "?"
        return d
    except Exception as e:
        die(f"Impossible de lire le certificat: {e}")

def new_ctx():
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
    ctx.check_hostname = False
    ctx.verify_mode    = ssl.CERT_NONE
    return ctx

def ztna_request(endpoint):
    """Effectue un cycle ZTNA complet : mTLS → PEP authorize → proxy HTTP."""
    req = json.dumps({
        "protocol_version": 1, "action": "connect",
        "resource": {"type": "http", "host": TARGET, "port": TARGET_PORT},
        "context": {"src_ip": "", "device_info": {}}
    }).encode()
    raw = socket.create_connection((GW_HOST, GW_PORT), timeout=30)
    with new_ctx().wrap_socket(raw) as conn:
        # --- Handshake ZTNA (ConnectRequest / ConnectResponse) ---
        conn.sendall(struct.pack(">I", len(req)) + req)
        rl = b""
        while len(rl) < 4: rl += conn.recv(4 - len(rl))
        ml = struct.unpack(">I", rl)[0]
        p  = b""
        while len(p) < ml: p += conn.recv(ml - len(p))
        resp = json.loads(p)
        if resp.get("decision") != "allow":
            return None, resp  # (None, deny_resp)
        # --- Tunnel TCP actif → requête HTTP ---
        conn.settimeout(8)
        conn.sendall(f"GET {endpoint} HTTP/1.0\r\nHost: {TARGET}\r\n\r\n".encode())
        data = b""
        try:
            while True:
                chunk = conn.recv(4096)
                if not chunk: break
                data += chunk
        except Exception:
            pass   # TLS close_notify absent ou timeout → body déjà reçu
        if not data:
            raise RuntimeError(f"Aucune réponse HTTP de {TARGET}:{TARGET_PORT}")
        parts = data.split(b"\r\n\r\n", 1)
        header = parts[0]
        body   = parts[1] if len(parts) > 1 else b"{}"
        return (header.splitlines()[0].decode(), json.loads(body), resp.get("decision_id","?")), None

# ─── Formateurs de réponse ────────────────────────────────────────────────────
def _fmt_body(endpoint, body):
    ep = endpoint.rstrip("/")

    # ── /api/status ──────────────────────────────────────────────────
    if ep in ("/api/status", "/api/status/") or ep == "":
        print(f"  {B}Service :{N}  {body.get('service','datavault')}  v{body.get('version','?')}")
        print(f"  {B}Hôte    :{N}  {G}{body.get('host','?')}{N}  "
              f"uptime={body.get('uptime_s','?')}s  "
              f"requêtes={body.get('requests_served','?')}")
        print(f"  {D}{body.get('timestamp','')[:19]}{N}")
        if body.get("message"):
            print(f"  {G}✓  {body['message']}{N}")

    # ── /api/vault/records (alias legacy: /api/assets) ───────────────
    elif "/vault/records" in ep or "/assets" in ep:
        classif = body.get("classification", "🔒 CONFIDENTIEL")
        records = body.get("records", body.get("assets", []))
        count   = body.get("count", len(records))
        src     = body.get("accessed_from", "?")
        print(f"  {Y}{B}{classif}{N}  —  {count} enregistrements")
        print(f"  {D}Hôte: {body.get('host','?')}  Accès depuis: {src}  "
              f"{body.get('timestamp','')[:19]}{N}")
        print(f"  {D}{body.get('notice','')}{N}")
        print()
        print(f"  {'ID':8s}  {'Classification':18s}  {'Titulaire':22s}  Titre")
        print(f"  {'─'*80}")
        for r in records:
            print(f"  {r.get('id','?'):8s}  {r.get('classification',''):18s}  "
                  f"{r.get('owner',''):22s}  {r.get('title','')}")

    # ── /api/vault/secrets (alias legacy: /api/secrets) ─────────────
    elif "/vault/secrets" in ep or "/secrets" in ep:
        classif = body.get("classification", "🔐 TOP SECRET")
        secrets = body.get("secrets", body.get("items", []))
        count   = body.get("count", len(secrets))
        src     = body.get("accessed_from", "?")
        print(f"  {R}{B}{classif}{N}  —  {count} secrets protégés")
        print(f"  {D}Hôte: {body.get('host','?')}  Accès depuis: {src}  "
              f"{body.get('timestamp','')[:19]}{N}")
        print(f"  {Y}⚠  {body.get('notice','')}{N}")
        if body.get("warning"):
            print(f"  {R}⚠  {body['warning']}{N}")
        print()
        print(f"  {'ID':8s}  {'Nom':28s}  {'Classification':20s}  Dernière rotation")
        print(f"  {'─'*80}")
        for s in secrets:
            print(f"  {s.get('id','?'):8s}  {R}{s.get('name',''):28s}{N}  "
                  f"{s.get('classification',''):20s}  {s.get('last_rotation','?')}")
            if s.get("value"):
                print(f"  {'':8s}  {D}valeur: {s['value']}{N}")

    # ── /api/whoami ──────────────────────────────────────────────────
    elif "/whoami" in ep:
        print(f"  {B}Connexion identifiée côté DataVault :{N}")
        print(f"  {'source_ip':14s}  {G}{body.get('source_ip','?')}{N}")
        print(f"  {'hôte':14s}  {body.get('host','?')}")
        print(f"  {'uptime':14s}  {body.get('uptime_s','?')}s")
        print(f"  {D}{body.get('note','')}{N}")

    # ── Réponse générique ────────────────────────────────────────────
    else:
        print(json.dumps(body, indent=2, ensure_ascii=False))

# ─── Résolution des alias d'endpoints ─────────────────────────────────────────
ENDPOINT_ALIASES = {
    "/api/assets":          "/api/vault/records",
    "/api/secrets":         "/api/vault/secrets",
    "/api/vault/records":   "/api/vault/records",
    "/api/vault/secrets":   "/api/vault/secrets",
    "/api/status":          "/api/status",
    "/api/whoami":          "/api/whoami",
}

def resolve_endpoint(raw):
    """Normalise un endpoint (avec ou sans /api/).  Retourne l'URL réelle."""
    # ex: "records" → "/api/vault/records", "secrets" → "/api/vault/secrets"
    shortnames = {
        "records":  "/api/vault/records",
        "secrets":  "/api/vault/secrets",
        "whoami":   "/api/whoami",
        "status":   "/api/status",
        "assets":   "/api/vault/records",
    }
    if raw in shortnames:
        return shortnames[raw]
    if not raw.startswith("/"):
        raw = "/" + raw
    return ENDPOINT_ALIASES.get(raw, raw)

# ─── Commandes ────────────────────────────────────────────────────────────────
def cmd_whoami():
    check_cert()
    c = cert_info()
    print(f"\n{B}  Identité ZTNA — Device Certificate{N}")
    print(f"  {'─'*50}")
    print(f"  {'utilisateur':14s}  {G}{c.get('cn','?')}{N}")
    print(f"  {'organisation':14s}  {c.get('org','?')}")
    print(f"  {'serial':14s}  {D}{c.get('serial','?')}{N}")
    print(f"  {'expiration':14s}  {c.get('not_after','?')}")
    print(f"  {'certificat':14s}  {D}{CERT_FILE}{N}")
    print(f"  {'gateway':14s}  {D}{GW_HOST}:{GW_PORT}{N}")
    print()

def cmd_status():
    check_cert()
    print(f"\n{B}  ZTNA Gateway + DataVault status{N}")
    print(f"  {'─'*50}")
    # Test TCP gateway
    try:
        s = socket.create_connection((GW_HOST, GW_PORT), timeout=5)
        s.close()
        print(f"  {C}gateway{N}     {G}joignable{N}    {GW_HOST}:{GW_PORT}")
    except Exception as e:
        print(f"  {C}gateway{N}     {R}injoignable{N}  {e}")
        sys.exit(1)
    # Test DataVault via tunnel ZTNA
    try:
        result, deny = ztna_request("/api/status")
        if deny:
            print(f"  {C}DataVault{N}   {R}accès refusé{N}  reason={deny.get('reason','?')}")
        else:
            status_line, body, dec_id = result
            h   = body.get("host","?")
            upt = body.get("uptime_s","?")
            srv = body.get("requests_served","?")
            print(f"  {C}DataVault{N}   {G}accessible{N}    {TARGET}:{TARGET_PORT}  "
                  f"host={h}  uptime={upt}s  requêtes={srv}")
            print(f"  {D}decision_id={dec_id}{N}")
    except Exception as e:
        print(f"  {C}DataVault{N}   {Y}erreur{N}  {e}")
    print()

def cmd_get(raw_endpoint):
    check_cert()
    endpoint = resolve_endpoint(raw_endpoint)
    print(f"\n{C}  {B}ztna get {endpoint}{N}")
    print(f"  {D}wan-client ──(mTLS)──► ztna-gw:{GW_PORT} ──(PEP)──► CP ──(TCP)──► {TARGET}:{TARGET_PORT}{N}")
    print()
    try:
        result, deny = ztna_request(endpoint)
        if deny:
            print(f"  {R}<── DENY  reason={deny.get('reason','?')}{N}")
            print(f"  {D}decision_id={deny.get('decision_id','?')}{N}\n")
            sys.exit(1)
        status_line, body, dec_id = result
        print(f"  {G}<── ALLOW  {status_line}{N}")
        print(f"  {D}decision_id={dec_id}{N}")
        print()
        _fmt_body(endpoint, body)
    except Exception as e:
        die(str(e))

def cmd_vault(sub):
    """Raccourci : ztna vault records / secrets / whoami"""
    mapping = {
        "records": "/api/vault/records",
        "secrets": "/api/vault/secrets",
        "whoami":  "/api/whoami",
        "status":  "/api/status",
    }
    ep = mapping.get(sub)
    if not ep:
        die(f"Sous-commande vault inconnue : {sub}\n"
            f"         Valeurs : records | secrets | whoami | status")
    cmd_get(ep)

def cmd_help():
    print(f"""
{B}ztna{N} — Zero Trust Network Access CLI

{B}Usage :{N}
  ztna whoami              Identité du device (certificat X.509 courant)
  ztna status              État du gateway ZTNA + DataVault
  ztna get <endpoint>      Accès à une ressource via le tunnel ZTNA
  ztna vault <sub>         Raccourci vers les endpoints DataVault

{B}Endpoints DataVault :{N}
  ztna get /api/status           Santé de l'API interne (DataVault)
  ztna get /api/vault/records    Enregistrements confidentiels
  ztna get /api/vault/secrets    Secrets critiques [TOP SECRET]
  ztna get /api/whoami           Identité de la connexion côté serveur

{B}Raccourcis ztna vault :{N}
  ztna vault records    →  /api/vault/records
  ztna vault secrets    →  /api/vault/secrets
  ztna vault whoami     →  /api/whoami
  ztna vault status     →  /api/status

{B}Chaque appel ztna get effectue :{N}
  1. Connexion mTLS vers ztna-gw:{GW_PORT} avec le device-cert
  2. Envoi du ConnectRequest → autorisation PEP (CP)
  3. Si ALLOW → tunnel TCP transparent vers {TARGET}:{TARGET_PORT}
  4. Requête HTTP GET sur l'API DataVault interne

{B}Config :{N}
  certificat  {CERT_FILE}
  gateway     {GW_HOST}:{GW_PORT}
  ressource   {TARGET}:{TARGET_PORT}
""")

# ─── Main ─────────────────────────────────────────────────────────────────────
cmd  = sys.argv[1] if len(sys.argv) > 1 else "help"
args = sys.argv[2:]

if   cmd == "whoami":                     cmd_whoami()
elif cmd in ("status", "ping"):           cmd_status()
elif cmd == "get":
    if not args:
        die("Usage: ztna get <endpoint>  (ex: ztna get /api/vault/records)")
    cmd_get(args[0])
elif cmd == "vault":
    if not args:
        die("Usage: ztna vault <sub>  (records | secrets | whoami | status)")
    cmd_vault(args[0])
elif cmd in ("help", "--help", "-h"):
    cmd_help()
else:
    print(f"  {R}Commande inconnue: {cmd}{N}")
    cmd_help()
    sys.exit(1)

