#!/usr/bin/env python3
"""
ZTNA Demo — Simulateur PostgreSQL interactif (psql-compatible).

Affiche l'animation de connexion identique à la démo orchestrée,
puis propose un vrai prompt SQL interactif soutenu par SQLite en mémoire.
Le résultat est formaté exactement comme psql.
"""

import sys
import os
import time
import sqlite3
import signal
import random
import textwrap

try:
    import readline
    readline.parse_and_bind("tab: complete")
except ImportError:
    pass

# ── ANSI ─────────────────────────────────────────────────────────────────────
RST      = "\033[0m"
BOLD     = "\033[1m"
DIM      = "\033[2m"
GREEN    = "\033[0;32m"
YELLOW   = "\033[1;33m"
CYAN     = "\033[0;36m"
BG_GREEN = "\033[42m"
FG_BLACK = "\033[30m"

# ── Constants ────────────────────────────────────────────────────────────────
DB_NAME    = "appdb"
DB_USER    = "alice"
LOCAL_PORT = "15432"
PROMPT     = f"{DB_NAME}=> "
PROMPT_C   = f"{DB_NAME}-> "

SESSION_UUID  = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
CERT_SERIAL   = "3A:2B:1C:0D:4E:5F:6A:7B"

NAMES = [
    "alice", "bob", "charlie", "diana", "eve", "frank", "grace", "heidi",
    "ivan", "judy", "karl", "liam", "mia", "noah", "olivia", "paul",
    "quinn", "rosa", "sam", "tina", "uma", "victor", "wendy", "xavier",
    "yara", "zane", "alex", "bella", "cyrus", "dara", "emil", "fiona",
    "gavin", "hana", "igor", "jade", "kent", "luna", "marco", "nadia",
    "oscar", "priya", "rami", "sara", "tomas", "ursula", "vince", "wanda",
]
GROUPS = [
    "ztna-admins", "ztna-admins,ztna-dba", "ztna-devs",
    "ztna-devs,ztna-dba", "ztna-ops", "ztna-ops,ztna-admins",
    "ztna-devs,ztna-ops", "ztna-dba",
]
ROLES = ["admin", "developer", "operator", "auditor", "user"]


# ── Simulated JSON log (matches demo style) ─────────────────────────────────
_TSEC = [0]

def _ts():
    _TSEC[0] += random.randint(1, 3)
    s = _TSEC[0]
    m, sec = 30 + s // 60, s % 60
    ms = random.randint(0, 999)
    return f"2026-03-09T15:{m:02d}:{sec:02d}.{ms:03d}+01:00"

def _jlog(level, msg, **kv):
    t = _ts()
    extra = "".join(f',"{k}":"{v}"' for k, v in kv.items())
    return f'{{"time":"{t}","level":"{level}","msg":"{msg}"{extra}}}'


# ── Database Setup ───────────────────────────────────────────────────────────

def _init_db():
    conn = sqlite3.connect(":memory:")
    conn.execute("PRAGMA journal_mode=WAL")
    c = conn.cursor()

    c.executescript("""
        CREATE TABLE users (
            id          INTEGER PRIMARY KEY,
            username    TEXT NOT NULL,
            email       TEXT,
            groups      TEXT,
            role        TEXT DEFAULT 'user',
            created_at  TEXT,
            last_login  TEXT,
            active      BOOLEAN DEFAULT 1
        );
        CREATE TABLE sessions (
            id              TEXT PRIMARY KEY,
            user_id         INTEGER,
            resource_name   TEXT,
            backend         TEXT,
            status          TEXT DEFAULT 'active',
            cert_serial     TEXT,
            started_at      TEXT,
            expires_at      TEXT,
            bytes_in        INTEGER DEFAULT 0,
            bytes_out       INTEGER DEFAULT 0,
            end_reason      TEXT
        );
        CREATE TABLE resources (
            id          INTEGER PRIMARY KEY,
            name        TEXT UNIQUE NOT NULL,
            type        TEXT NOT NULL,
            backend     TEXT NOT NULL,
            access_mode TEXT,
            description TEXT,
            active      BOOLEAN DEFAULT 1
        );
        CREATE TABLE policies (
            id              INTEGER PRIMARY KEY,
            name            TEXT NOT NULL,
            effect          TEXT DEFAULT 'allow',
            subject_groups  TEXT,
            action          TEXT DEFAULT 'connect',
            resource_type   TEXT,
            conditions      TEXT,
            priority        INTEGER DEFAULT 0
        );
        CREATE TABLE audit_log (
            id      INTEGER PRIMARY KEY AUTOINCREMENT,
            ts      TEXT DEFAULT CURRENT_TIMESTAMP,
            event   TEXT,
            subject TEXT,
            resource TEXT,
            action  TEXT,
            decision TEXT,
            detail  TEXT
        );
    """)

    # ── 1247 users ───────────────────────────────────────────────────────
    random.seed(42)
    rows = []
    for i in range(1, 1248):
        n = NAMES[i % len(NAMES)]
        sfx = "" if i <= len(NAMES) else str(i // len(NAMES))
        uname = f"{n}{sfx}"
        email = f"{uname}@corp.example.com"
        grp   = GROUPS[i % len(GROUPS)]
        role  = ROLES[i % len(ROLES)]
        mo    = random.randint(1, 3)
        day   = random.randint(1, 28)
        cr    = f"2026-{mo:02d}-{day:02d} {random.randint(8,18):02d}:{random.randint(0,59):02d}:00"
        ld    = random.randint(1, 9)
        ll    = f"2026-03-{ld:02d} {random.randint(8,18):02d}:{random.randint(0,59):02d}:00"
        active = 1 if random.random() > 0.05 else 0
        rows.append((i, uname, email, grp, role, cr, ll, active))
    c.executemany("INSERT INTO users VALUES (?,?,?,?,?,?,?,?)", rows)

    # ── resources ────────────────────────────────────────────────────────
    for r in [
        (1, "ssh-dev-01",        "ssh", "10.10.30.15:22",   "ssh-cert",   "Serveur SSH dev",       1),
        (2, "grafana-internal",  "web", "10.10.30.15:3000", "http-proxy", "Grafana interne",       1),
        (3, "pg-staging",        "db",  "10.10.30.15:5432", "tcp-tunnel", "PostgreSQL staging",    1),
        (4, "redis-cache",       "db",  "10.10.30.15:6379", "tcp-tunnel", "Redis cache",           1),
        (5, "api-gateway",       "web", "10.10.30.15:8080", "http-proxy", "API Gateway interne",   1),
    ]:
        c.execute("INSERT INTO resources VALUES (?,?,?,?,?,?,?)", r)

    # ── policies ─────────────────────────────────────────────────────────
    for p in [
        (1, "allow-admins-ssh",  "allow", "ztna-admins", "connect", "ssh", "hours:08-22,trust:medium", 10),
        (2, "allow-admins-web",  "allow", "ztna-admins", "connect", "web", "hours:08-22",              10),
        (3, "allow-dba-db",      "allow", "ztna-dba",    "connect", "db",  "hours:08-22,trust:medium", 10),
        (4, "allow-devs-ssh",    "allow", "ztna-devs",   "connect", "ssh", "hours:09-20",               5),
        (5, "deny-all-default",  "deny",  "*",           "*",       "*",   None,                        0),
    ]:
        c.execute("INSERT INTO policies VALUES (?,?,?,?,?,?,?,?)", p)

    # ── sessions ─────────────────────────────────────────────────────────
    c.execute(
        "INSERT INTO sessions VALUES (?,?,?,?,?,?,?,?,?,?,?)",
        (SESSION_UUID, 1, "pg-staging", "10.10.30.15:5432", "active",
         CERT_SERIAL, "2026-03-09 15:30:42", "2026-03-09 16:30:42",
         2947, 8234, None),
    )

    # ── audit_log ────────────────────────────────────────────────────────
    for a in [
        (None, "2026-03-09 15:30:40", "auth.oidc",       "alice", None,         "login",   "success", "Keycloak ROPC"),
        (None, "2026-03-09 15:30:41", "cert.issue",       "alice", None,         "cert",    "success", "Device CA signed"),
        (None, "2026-03-09 15:30:42", "access.connect",   "alice", "pg-staging", "connect", "allow",   "rule:allow-dba-db"),
    ]:
        c.execute("INSERT INTO audit_log VALUES (?,?,?,?,?,?,?,?)", a)

    conn.commit()
    return conn


# ── Output Formatting (psql-compatible) ──────────────────────────────────────

def _fmt_table(cursor):
    """Format SELECT results like psql."""
    if cursor.description is None:
        return None
    cols = [d[0] for d in cursor.description]
    rows = cursor.fetchall()
    srows = [[str(v) if v is not None else "" for v in r] for r in rows]

    widths = [len(c) for c in cols]
    for sr in srows:
        for i, v in enumerate(sr):
            if i < len(widths):
                widths[i] = max(widths[i], len(v))

    lines = []
    lines.append(" " + " | ".join(c.ljust(w) for c, w in zip(cols, widths)))
    lines.append("-" + "-+-".join("-" * w for w in widths) + "-")
    for sr in srows:
        lines.append(" " + " | ".join(v.ljust(w) for v, w in zip(sr, widths)))
    n = len(rows)
    lines.append(f"({n} {'row' if n == 1 else 'rows'})")
    return "\n".join(lines)


def _handle_meta(cmd, conn):
    """Handle psql backslash commands. Returns None to quit."""
    parts = cmd.strip().split(None, 1)
    main = parts[0]
    arg  = parts[1].strip() if len(parts) > 1 else None

    if main in ("\\q", "\\quit"):
        return None

    if main == "\\dt":
        cur = conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
        )
        tables = cur.fetchall()
        lines = ["          List of relations"]
        lines.append(" Schema  |     Name      | Type  | Owner ")
        lines.append("---------+---------------+-------+-------")
        for (name,) in tables:
            lines.append(f" public  | {name:<13} | table | {DB_USER}")
        lines.append(f"({len(tables)} rows)")
        return "\n".join(lines)

    if main == "\\d" and arg:
        tname = arg.strip().rstrip(";")
        try:
            cur = conn.execute(f"PRAGMA table_info([{tname}])")
            info = cur.fetchall()
            if not info:
                return f'Did not find any relation named "{tname}".'
            lines = [f'                Table "public.{tname}"']
            lines.append("   Column      |   Type    | Nullable |  Default")
            lines.append("---------------+-----------+----------+----------")
            for _, name, dtype, notnull, default, _ in info:
                nn = "not null" if notnull else ""
                dv = str(default) if default is not None else ""
                lines.append(
                    f" {name:<13} | {(dtype or 'text').lower():<9} | {nn:<8} | {dv}"
                )
            return "\n".join(lines)
        except Exception:
            return f'Did not find any relation named "{tname}".'

    if main in ("\\l", "\\list"):
        lines = ["                           List of databases"]
        lines.append("   Name    | Owner  | Encoding |  Collate    |   Ctype")
        lines.append("-----------+--------+----------+-------------+-------------")
        lines.append(f" {'appdb':<9} | {'alice':<6} | UTF8     | en_US.UTF-8 | en_US.UTF-8")
        lines.append(f" {'postgres':<9} | {'alice':<6} | UTF8     | en_US.UTF-8 | en_US.UTF-8")
        lines.append("(2 rows)")
        return "\n".join(lines)

    if main == "\\conninfo":
        return (
            f'You are connected to database "{DB_NAME}" as user "{DB_USER}" '
            f'on host "localhost" (address "127.0.0.1") at port "{LOCAL_PORT}".\n'
            f"Connection secured via ZTNA mTLS tunnel through Gateway 10.10.10.20:8443."
        )

    if main in ("\\?", "\\help"):
        return textwrap.dedent("""\
            General
              \\q              quit
              \\dt             list tables
              \\d TABLE        describe table
              \\l              list databases
              \\conninfo       connection info
              \\?              this help""")

    return f"Invalid command {main}. Try \\? for help."


def _exec_sql(sql, conn):
    """Execute SQL and return psql-formatted output."""
    clean = sql.strip().rstrip(";").strip()
    if not clean:
        return ""
    # Rewrite count(*) → count(*) AS count to match psql column naming
    import re
    clean = re.sub(
        r'\bcount\s*\(\s*\*\s*\)',
        'count(*) AS count',
        clean,
        flags=re.IGNORECASE,
    )
    try:
        cur = conn.execute(clean)
        conn.commit()
        if cur.description:
            return _fmt_table(cur)
        n = cur.rowcount if cur.rowcount >= 0 else 0
        kw = clean.upper().split()[0] if clean.split() else ""
        if kw == "INSERT":
            return f"INSERT 0 {n}"
        if kw == "UPDATE":
            return f"UPDATE {n}"
        if kw == "DELETE":
            return f"DELETE {n}"
        if kw == "CREATE":
            return "CREATE TABLE" if "TABLE" in clean.upper() else "CREATE"
        if kw == "DROP":
            return "DROP TABLE" if "TABLE" in clean.upper() else "DROP"
        if kw == "ALTER":
            return "ALTER TABLE"
        return ""
    except sqlite3.Error as e:
        return f"ERROR:  {e}"


# ── Intro Animation (matches demo-interactive step 9) ───────────────────────

def _animate_intro():
    """
    Reprint the exact same visual the orchestrated demo shows,
    then leave the cursor at the appdb=> prompt.
    """
    sys.stdout.write("\033[2J\033[H")  # clear + home
    sys.stdout.flush()

    # ── Header ──
    hdr = (
        f"{BG_GREEN}{FG_BLACK}{BOLD}"
        f" \u25cf {'CLIENT CLI':<18} {'10.10.10.10':<16} {'ACTIF':>14} "
        f"{RST}"
    )
    print(hdr)
    print()
    time.sleep(0.2)

    j = _jlog("INFO", "réponse Gateway reçue",
              status="allow", session_id=SESSION_UUID)
    print(f"  {j}")
    print()
    time.sleep(0.2)

    print(f"  {GREEN}{BOLD}  \u2713 Accès autorisé — tunnel proxy actif{RST}")
    print()
    time.sleep(0.4)

    # ── psql connection ──
    print(f"  $ {BOLD}psql -h localhost -p {LOCAL_PORT} -U {DB_USER} -d {DB_NAME}{RST}")
    time.sleep(0.15)
    print(f"  {DIM}  Password for user {DB_USER}: ****{RST}")
    time.sleep(0.15)
    print("  psql (15.4)")
    print('  Type "help" for help.')
    print()
    time.sleep(0.3)

    # ── Example query ──
    print(f"{PROMPT}{BOLD}SELECT count(*) FROM users;{RST}")
    time.sleep(0.2)
    print(" count ")
    print("-------")
    print("  1247")
    print("(1 row)")
    print()


# ── Interactive Loop ─────────────────────────────────────────────────────────

def main():
    signal.signal(signal.SIGINT, lambda _s, _f: None)

    conn = _init_db()
    _animate_intro()

    buf = ""
    while True:
        try:
            prompt = PROMPT if not buf else PROMPT_C
            line = input(prompt)
        except EOFError:
            print()
            break
        except KeyboardInterrupt:
            print()
            buf = ""
            continue

        stripped = line.strip()

        # ── Meta-commands ──
        if not buf and stripped.startswith("\\"):
            result = _handle_meta(stripped, conn)
            if result is None:
                break
            print(result)
            continue

        # ── Help / exit aliases ──
        if not buf and stripped.lower() == "help":
            print("You are using psql, the PostgreSQL interactive terminal.")
            print("Type:  \\? for help with psql commands")
            print("       \\q to quit")
            continue
        if not buf and stripped.lower() in ("exit", "quit"):
            break

        # ── Accumulate SQL until semicolon ──
        buf += (" " if buf else "") + line
        if ";" in buf:
            result = _exec_sql(buf, conn)
            if result:
                print(result)
            buf = ""

    conn.close()
    print(f"{DIM}Connexion ZTNA fermée.{RST}")


if __name__ == "__main__":
    main()
