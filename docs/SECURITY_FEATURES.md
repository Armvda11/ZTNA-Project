# Fonctionnalités de Sécurité Industrielles — ZTNA Gateway

> Ce document recense les fonctionnalités de sécurité de niveau industriel
> implémentées dans la Gateway ZTNA, avec justification technique par
> rapport aux standards du marché (OpenZiti, Cloudflare Access, BeyondCorp).

---

## Table des matières

1. [CRL Auto-Refresh](#1-crl-auto-refresh)
2. [Protection SSRF](#2-protection-ssrf)
3. [Session Manager](#3-session-manager)
4. [Decision Cache avec mode CP-Down](#4-decision-cache-avec-mode-cp-down)
5. [Heartbeat Gateway→CP](#5-heartbeat-gatewaycp)
6. [Télémétrie de sessions](#6-télémétrie-de-sessions)
7. [MaxBytesReader](#7-maxbytesreader)
8. [Sanitisation d'erreurs](#8-sanitisation-derreurs)
9. [Architecture hexagonale](#9-architecture-hexagonale)
10. [Graceful Shutdown](#10-graceful-shutdown)
11. [Couverture de tests](#11-couverture-de-tests)

---

## 1. CRL Auto-Refresh

**Fichier:** `gateway/internal/infra/revocation/crl.go`

**Problème:** Un certificat révoqué restait accepté tant que la CRL n'était
pas rechargée manuellement.

**Solution:** Le CRL Store interroge périodiquement le Control Plane
(`GET /pki/device-ca/crl`) et met à jour la liste des numéros de série
révoqués. Le parsing supporte les formats PEM et DER via
`x509.ParseRevocationList`.

**Intégration:**
- Le listener mTLS vérifie la CRL **après le handshake TLS** et avant
  de dispatcher vers le handler (double check : handshake + application layer).
- Le handler CONNECT vérifie à nouveau avant l'autorisation (defense in depth).

**Comparable à:** Cloudflare Access — CRL polling, BeyondCorp — continuous
certificate validation.

---

## 2. Protection SSRF

**Fichier:** `gateway/internal/infra/proxy/tcp.go` → `validateTarget()`

**Problème:** Sans validation, un attaquant avec un certificat valide
pouvait demander un proxy vers `127.0.0.1`, `169.254.169.254` (cloud
metadata), ou d'autres adresses sensibles.

**Solution:** Résolution DNS + validation de chaque IP résolue :
- Loopback (`127.0.0.0/8`, `::1`)
- Link-local (`169.254.0.0/16`, `fe80::/10`)
- Multicast, broadcast, unspecified
- Cloud metadata (`169.254.169.254`)
- Ports invalides (< 1 ou > 65535)

**Comparable à:** Zscaler ZPA — SSRF protection on connector,
AWS API Gateway — private link restrictions.

---

## 3. Session Manager

**Fichier:** `gateway/internal/infra/session/manager.go`

**Fonctionnalités:**

| Feature | Description |
|---------|-------------|
| **UUID v4** | Chaque session reçoit un identifiant unique (crypto/rand) |
| **TTL enforcement** | `ExpiresAt` calculé depuis la décision du CP |
| **Per-subject limits** | Max 10 sessions simultanées par utilisateur (configurable) |
| **Admin kill** | `KillSession()` appelle `CancelFunc` + supprime de la map |
| **Garbage collector** | Goroutine périodique qui reap les sessions expirées |
| **End metrics** | Bytes in/out, durée, raison de fin (normal/ttl_expired/admin_kill) |
| **Thread-safe** | `sync.RWMutex` pour accès concurrent |

**Comparable à:** OpenZiti — session TTL + forced termination,
BeyondCorp — continuous session evaluation.

---

## 4. Decision Cache avec mode CP-Down

**Fichier:** `gateway/internal/infra/cache/` + `handler.go`

**Fonctionnement:**
1. Clé de cache : `sub|action|type|host|port`
2. Lookup cache → hit → retourne directement (court-circuite le CP)
3. Miss → appel CP → stocke dans cache avec TTL
4. Si CP injoignable → mode dégradé configurable :
   - `deny` (défaut) : refuse tout
   - `allow_cached` : autorise si une décision récente existe en cache

**Comparable à:** Cloudflare Access — edge caching of auth decisions,
Zscaler ZPA — local policy cache on connector.

---

## 5. Heartbeat Gateway→CP

**Fichier:** `gateway/internal/infra/heartbeat/client.go`

Le PEP envoie un heartbeat périodique (`POST /api/v1/pep/heartbeat`)
avec son ID et sa version. Le CP met à jour le `last_seen` et peut
détecter les passerelles déconnectées.

**Intervalle par défaut:** 30 secondes (configurable).

**Comparable à:** OpenZiti — edge router heartbeat,
Zscaler ZPA — connector health signaling.

---

## 6. Télémétrie de sessions

**Fichier:** `gateway/internal/infra/telemetry/client.go`

Notifications fire-and-forget vers le CP :
- `POST /api/v1/pep/sessions/start` à l'ouverture
- `POST /api/v1/pep/sessions/end` à la fermeture (avec bytes, durée, raison)

Les appels sont non-bloquants (goroutines avec timeout 5s).

**Comparable à:** BeyondCorp — session telemetry to central logging,
Cloudflare Access — access logs.

---

## 7. MaxBytesReader

**Fichiers:** `control-plane/internal/api/handlers/pep_*.go`

Protection contre les requêtes surdimensionnées :
`r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` (1 MB max).

Appliqué sur tous les endpoints PEP :
- `/api/v1/pep/authorize`
- `/api/v1/pep/sessions/start`
- `/api/v1/pep/heartbeat`

---

## 8. Sanitisation d'erreurs

**Fichier:** `control-plane/internal/api/httputil/httputil.go`

- `errors.Is()` au lieu de `==` pour supporter les erreurs wrappées.
- Les erreurs 500 retournent `"erreur interne du serveur"` au lieu de
  `err.Error()` (pas de fuite d'information).
- Les erreurs 4xx retournent le message d'erreur domaine (utile au client).

---

## 9. Architecture hexagonale

**Fichier:** `gateway/internal/core/ports/ports.go`

Interfaces définies pour chaque composant :

```go
type Authorizer interface { ... }
type Proxy interface { ... }
type SessionManager interface { ... }
type RevocationChecker interface { ... }
type DecisionCache interface { ... }
type SessionTelemetry interface { ... }
type ConnectionHandler interface { ... }
```

Permet :
- Tests unitaires avec mocks
- Remplacement d'implémentation sans toucher au handler
- Séparation claire domaine/infrastructure

---

## 10. Graceful Shutdown

**Fichier:** `gateway/internal/bootstrap/app.go` → `Close()`

Séquence d'arrêt :
1. Fermeture du listener (plus de nouvelles connexions)
2. Drain : attente des sessions actives avec polling 500ms
3. Si deadline dépassé : kill forcé des sessions restantes
4. Vidage du cache de décisions
5. Log de confirmation

Le `main.go` intercepte `SIGINT`/`SIGTERM` via `signal.NotifyContext`
avec un timeout de 10 secondes.

**Fichier:** `gateway/internal/usecase/lifecycle/lifecycle.go`

Orchestrateur LIFO pour arrêt de composants multiples.

---

## 11. Couverture de tests

| Composant | Packages | Tests | Résultat |
|-----------|----------|-------|----------|
| Gateway | 13 | 86 | ✅ 100% pass |
| Control Plane | 5 | 31 | ✅ 100% pass |
| **Total** | **18** | **104+** | **✅ 0 failure** |

### Tests notables

- **SSRF Protection** : 9 cas (loopback, link-local, metadata, multicast, ports)
- **Session Manager** : registration, limits, expiration/GC, cleanup, metrics,
  concurrent access, admin kill
- **Proxy** : bidirectional relay, timeout, context cancellation, byte counting
- **CRL** : store replace, snapshot isolation
- **Policy Engine** : 12 cas ABAC (user, group, sub, wildcard, resource type)

---

## Comparaison avec les solutions industrielles

| Feature | Ce projet | OpenZiti | Cloudflare Access | Zscaler ZPA |
|---------|-----------|----------|-------------------|-------------|
| mTLS | ✅ TLS 1.3 | ✅ | ✅ | ✅ |
| CRL/OCSP | ✅ CRL auto | ✅ OCSP | ✅ | ✅ |
| Session TTL | ✅ | ✅ | ✅ | ✅ |
| SSRF protection | ✅ | ✅ | ✅ | ✅ |
| Decision cache | ✅ | ✅ | ✅ edge | ✅ |
| CP-down resilience | ✅ | ✅ | ✅ | ✅ |
| Heartbeat | ✅ | ✅ | ✅ | ✅ |
| Session telemetry | ✅ | ✅ | ✅ | ✅ |
| Graceful shutdown | ✅ | ✅ | ✅ | ✅ |
| Device posture | ⬜ framework | ✅ | ✅ | ✅ |
| L7 inspection | ⬜ TCP only | ✅ | ✅ HTTP | ✅ |
| Multi-region | ⬜ single | ✅ | ✅ global | ✅ |

**Niveau atteint : Tier 1 complet, Tier 2 à ~70%**
