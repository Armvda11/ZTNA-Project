#!/usr/bin/env python3
"""
ztna — Zero Trust Network Access CLI
Outil client installé sur wan-client par l'étape 3 de la démo.

Usage:
  ztna whoami              Identité du device (certificat X.509)
  ztna status              État du gateway et de l'API interne
  ztna get <endpoint>      Accès à une ressource via tunnel ZTNA
"""
import ssl, socket, struct, json, sys, os, subprocess, re

# ─── Config (lue depuis /tmp/ztna-demo/) ──────────────────────────────────────
CONF_DIR  = "/tmp/ztna-demo"
GW_PORT   = 4433
CERT_FILE = f"{CONF_DIR}/device.crt"
KEY_FILE  = f"{CONF_DIR}/device.key"
TARGET    = "lan-app"
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
N  = "\033[0m"      # reset

# ─── Helpers ──────────────────────────────────────────────────────────────────
def die(msg):
    print(f"\n  {R}[error]{N} {msg}\n", file=sys.stderr)
    sys.exit(1)

def check_cert():
    if not os.path.exists(CERT_FILE):
        die(f"Certificat introuvable: {CERT_FILE}\n         Lancez d'abord l'étape 3 : make demo")

def cert_info():
    try:
        out = subprocess.check_output(
            ["openssl","x509","-noout","-subject","-serial","-dates","-in",CERT_FILE],
            stderr=subprocess.DEVNULL).decode()
        d = {}
        for line in out.splitlines():
            k, _, v = line.partition("=")
            k = k.strip().lower()
            if   "subject" in k: d["subject"]    = v.strip()
            elif "serial"  in k: d["serial"]     = v.strip().lower()
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
        header, body = data.split(b"\r\n\r\n", 1)
        return (header.splitlines()[0].decode(), json.loads(body), resp.get("decision_id","?")), None

# ─── Formateurs de réponse ────────────────────────────────────────────────────
def _fmt_body(endpoint, body):
    if "/status" in endpoint:
        svc = body.get("services", {})
        print(f"  {B}Service :{N}  {body.get('host')}  "
              f"uptime={body.get('uptime_s')}s  {D}{body.get('timestamp','')[:19]}{N}")
        for k, v in svc.items():
            col = G if v == "up" else R
            print(f"    {col}{k:12s}  {v}{N}")

    elif "/assets" in endpoint:
        assets = body.get("assets", [])
        print(f"  {B}Inventaire — {body.get('count')} assets (réseau LAN privé){N}")
        print(f"  {'ID':8s}  {'Nom':22s}  {'Type':14s}  {'Env':12s}  Statut")
        print(f"  {'─'*70}")
        for a in assets:
            col = G if a.get("status") in ("running","active") else Y
            print(f"  {col}{a['id']:8s}  {a['name']:22s}  {a['type']:14s}  "
                  f"{a['env']:12s}  {a['status']}{N}")

    elif "/secrets" in endpoint:
        print(f"  {Y}{B}{body.get('notice','CONFIDENTIEL')}{N}")
        secrets = body.get("secrets", [])
        print(f"  {body.get('count')} secrets protégés :")
        print(f"  {'Clé':35s}  {'Rotation':12s}  Propriétaire")
        print(f"  {'─'*65}")
        for s in secrets:
            print(f"  {s['key']:35s}  {s['rotation']:12s}  {s['owner']}")

    else:
        print(json.dumps(body, indent=2, ensure_ascii=False))

# ─── Commandes ────────────────────────────────────────────────────────────────
def cmd_whoami():
    check_cert()
    c = cert_info()
    print(f"\n{B}  Identité ZTNA — Device Certificate{N}")
    print(f"  {'─'*50}")
    print(f"  {'utilisateur':14s}  {G}{c.get('cn','?')}{N}")
    print(f"  {'groupe':14s}  {c.get('org','?')}")
    print(f"  {'serial':14s}  {D}{c.get('serial','?')}{N}")
    print(f"  {'expiration':14s}  {c.get('not_after','?')}")
    print(f"  {'certificat':14s}  {D}{CERT_FILE}{N}")
    print(f"  {'gateway':14s}  {D}{GW_HOST}:{GW_PORT}{N}")
    print()

def cmd_status():
    check_cert()
    print(f"\n{B}  ZTNA Gateway status{N}")
    print(f"  {'─'*50}")
    # Test TCP gateway
    try:
        s = socket.create_connection((GW_HOST, GW_PORT), timeout=5)
        s.close()
        print(f"  gateway     {G}joignable{N}   {GW_HOST}:{GW_PORT}")
    except Exception as e:
        print(f"  gateway     {R}injoignable{N}  {e}")
        sys.exit(1)
    # Test API interne via tunnel ZTNA
    try:
        result, deny = ztna_request("/api/status")
        if deny:
            print(f"  api interne {R}acces refuse{N}  reason={deny.get('reason','?')}")
        else:
            status_line, body, dec_id = result
            print(f"  api interne {G}accessible{N}    {TARGET}:{TARGET_PORT}  "
                  f"uptime={body.get('uptime_s')}s")
            svc = body.get("services", {})
            for k, v in svc.items():
                col = G if v == "up" else R
                print(f"    {col}{k:12s}  {v}{N}")
    except Exception as e:
        print(f"  api interne {Y}erreur{N}  {e}")
    print()

def cmd_get(endpoint):
    check_cert()
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

def cmd_help():
    print(f"""
{B}ztna{N} — Zero Trust Network Access CLI

{B}Usage :{N}
  ztna whoami              Identité du device (certificat X.509 courant)
  ztna status              État du gateway ZTNA et de l'API interne
  ztna get <endpoint>      Accès à une ressource via le tunnel ZTNA

{B}Ressources disponibles :{N}
  ztna get /api/status     État du service interne
  ztna get /api/assets     Inventaire des assets (serveurs, postes, réseau)
  ztna get /api/secrets    Secrets & credentials [CONFIDENTIEL]

{B}Chaque appel ztna get effectue :{N}
  1. Connexion mTLS vers ztna-gw:{GW_PORT} avec le device-cert
  2. Envoi du ConnectRequest → autorisation PEP (CP)
  3. Si ALLOW → tunnel TCP transparent vers {TARGET}:{TARGET_PORT}
  4. Requête HTTP sur la ressource interne

{B}Config :{N}
  certificat  {CERT_FILE}
  gateway     {GW_HOST}:{GW_PORT}
  ressource   {TARGET}:{TARGET_PORT}
""")

# ─── Main ─────────────────────────────────────────────────────────────────────
cmd = sys.argv[1] if len(sys.argv) > 1 else "help"

if   cmd == "whoami":           cmd_whoami()
elif cmd in ("status","ping"):  cmd_status()
elif cmd == "get":
    if len(sys.argv) < 3:
        die("Usage: ztna get <endpoint>  (ex: ztna get /api/assets)")
    cmd_get(sys.argv[2])
elif cmd in ("help","--help","-h"):
    cmd_help()
else:
    print(f"  {R}Commande inconnue: {cmd}{N}")
    cmd_help()
    sys.exit(1)
