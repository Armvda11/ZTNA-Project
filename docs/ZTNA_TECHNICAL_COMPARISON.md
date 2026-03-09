# ZTNA Technical Comparison — Industrial-Grade Assessment

> **Date**: 9 mars 2026  
> **Scope**: OpenZiti, Cloudflare Access, Google BeyondCorp, Zscaler ZPA, and this project  
> **Purpose**: Concrete technical comparison for expert jury preparation

---

## Table of Contents

- [A. Feature Comparison Matrix](#a-feature-comparison-matrix)
- [B. Top 15 Features an Expert Jury Would Test](#b-top-15-features-an-expert-jury-would-test)
- [C. Minimum Viable Feature Set for "Industrial Grade"](#c-minimum-viable-feature-set-for-industrial-grade)
- [D. Implementation Recommendations (Go Code-Level Guidance)](#d-implementation-recommendations)

---

## A. Feature Comparison Matrix

### Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Fully implemented and production-ready |
| ⚠️ | Partially implemented or stub/TODO exists |
| ❌ | Not implemented |
| N/A | Not applicable to that architecture |

---

### A.1 Identity & Authentication

| Feature | OpenZiti | Cloudflare Access | BeyondCorp | Zscaler ZPA | Our Project |
|---------|----------|-------------------|------------|-------------|-------------|
| **OIDC/SAML authentication** | ✅ External IdP (any OIDC/SAML via ext-jwt-signer) | ✅ Identity-aware proxy; supports Okta, Azure AD, Google, GitHub, SAML, OIDC | ✅ Google Identity via context-aware access | ✅ SAML 2.0 + OIDC; IdP proxy with SCIM provisioning | ✅ OIDC via Keycloak (RS256 JWKS offline validation, `aud_or_azp` mode) |
| **Enrollment / device identity** | ✅ One-time-token (OTT) enrollment; each identity gets unique x509 cert from built-in PKI; identity types: device, service, router | ✅ WARP client enrollment; device serial + MDM integration; unique device UUID | ✅ Device inventory DB; X.509 cert per device issued by enterprise CA; device must be in inventory | ✅ App connector & client connector auto-enrollment; cloud-hosted identity | ⚠️ X.509 device certs issued by built-in Device CA (`POST /api/v1/credentials/device-cert`); no MDM integration; no device inventory DB |
| **mTLS tunnels** | ✅ All overlay traffic is mTLS 1.3; libsodium session keys rotated per-session; smart routing via edge routers | ✅ Cloudflare Tunnel (cloudflared): outbound-only TLS 1.3 from connector to Cloudflare edge; no inbound ports needed | ❌ Access proxy model (HTTPS); no client-side tunnel by default; uses context-aware TLS termination at proxy | ✅ Inside-out connections from App Connector via TLS 1.3; ZPA broker mediates session | ✅ Gateway mTLS listener on `:4433` with `RequireAndVerifyClientCert` (TLS 1.3); client presents device cert |
| **Multi-factor auth (MFA)** | ❌ Delegated to IdP | ✅ Supports IdP MFA; can add CF Access-level step-up (WARP posture) | ✅ Delegated to Google; fine-grained reauth policies per access level | ✅ Delegated to IdP via SAML assertion | ❌ Planned (Phase 2 roadmap: TOTP/U2F) |
| **Service account / machine identity** | ✅ First-class: services and routers each get PKI identities | ✅ Service tokens (client ID + secret) for non-human access | ⚠️ Service accounts via Google IAM; robot accounts | ✅ Machine tunnel via ZPA API; connector identity | ⚠️ PEP auth by token (`X-PEP-ID`/`X-PEP-TOKEN`) or mTLS for gateway-to-CP; no dedicated service identity framework |

### A.2 Authorization & Policy Engine

| Feature | OpenZiti | Cloudflare Access | BeyondCorp | Zscaler ZPA | Our Project |
|---------|----------|-------------------|------------|-------------|-------------|
| **Policy model** | ✅ Service policies (Bind + Dial) with `#attributes`-based matching; posture checks inline in policy | ✅ Access policies per application: Allow/Block/Bypass with include/exclude/require rule groups; evaluated top-down | ✅ Access Levels (trust tiers) composed of conditions (device, network, geo, time); bound to resources via IAP | ✅ Policy per application segment; conditions on user, group, connector group, SAML attrs, posture profile | ✅ Versioned rules with allow/deny, subject match (`group:X`, wildcard), default-deny, ordered evaluation |
| **Attribute-based access (ABAC)** | ✅ Identity attributes + service attributes; role attributes matched at dial time | ✅ Include/exclude on IdP groups, emails, IP ranges, country, device posture, auth method, mTLS cert | ✅ Access Levels combine device, IP, geo, time-of-day, authentication strength | ✅ SAML attributes, posture profiles, geo-location, time-of-day | ⚠️ Group-based + resource-type matching; no IP/geo/time conditions in policy rules yet |
| **Continuous authorization** | ✅ Posture checks re-evaluated on configurable intervals (e.g., every 5 min); fail → session torn down | ✅ Session duration per app (token TTL); re-auth forced on expiry; WARP posture re-checked continuously | ✅ Continuous evaluation: device trust + context re-evaluated; IAP checks on each request (HTTP) | ✅ Continuous evaluation via ZIA/ZPA telemetry; posture re-assessment; session timeout | ❌ Authorization only at connect time; no periodic re-evaluation during session; TTL on decision advertised but not enforced on gateway side |
| **Policy versioning** | ❌ No built-in versioning; managed via automation/GitOps | ❌ Audit log of changes only | ❌ Version history via Cloud Console | ❌ Managed via ZPA Admin Portal | ✅ Explicit version IDs; create/activate workflow; `GET /api/v1/admin/policies/active`; seed on startup |
| **Dynamic policy push** | ✅ Controller pushes policy updates to edge routers in real-time; sessions re-evaluated | ❌ Policy changes propagate within ~60s across edge; no push to client | ⚠️ IAP evaluates per-request; policy change effective immediately for HTTP but not for existing TCP sessions | ✅ Policy Engine updates propagated to brokers | ❌ Gateway polls CP (heartbeat); no push channel; stale decisions possible between heartbeat intervals |

### A.3 Certificate Management & Revocation

| Feature | OpenZiti | Cloudflare Access | BeyondCorp | Zscaler ZPA | Our Project |
|---------|----------|-------------------|------------|-------------|-------------|
| **PKI / Certificate Authority** | ✅ Built-in CA (edge enrollment CA + intermediate signing CA); x509 for all identities; optional external CA integration | ✅ CF-managed PKI; auto-provisioned origin certs; mTLS support with customer CA or CF CA | ✅ Enterprise CA (Google or customer); device certs for trust verification | ✅ ZPA cloud broker issues session certs; connectors use auto-enrolled certs | ✅ SSH CA (Ed25519, auto-generated at startup) + Device CA (X.509 for mTLS); both built-in |
| **CRL / OCSP revocation** | ✅ Controller maintains enrollment revocation; revoked identities cannot re-enroll or connect | ⚠️ mTLS revocation via CRL upload; no OCSP; main revocation is at identity/session level | ⚠️ Device trust revocation via device inventory removal; CRL for enterprise certs | ✅ Certificate revocation managed by cloud broker; instant session termination | ✅ CRL endpoint (`GET /pki/device-ca/crl`); gateway periodic CRL refresh (default 60s); `strict_revocation` mode; `KillRevoked` on gateway kills active sessions of revoked certs |
| **Real-time revocation** | ✅ Controller terminates sessions of revoked identities; edge routers enforce immediately | ✅ Admin can revoke user sessions instantly from dashboard; propagation <1 min | ⚠️ Device removal from inventory effective on next request; existing TCP sessions may persist | ✅ Instant session teardown on revocation via broker | ⚠️ CRL-based with polling interval (60s default); not instant but sub-minute; `DELETE /api/v1/admin/device-certs/{serial}` adds to CRL |
| **mTLS cert rotation without drop** | ✅ Session key rotation in overlay; no connection drop; identity cert renewed via re-enrollment | ✅ Origin cert renewal handled by CF edge; transparent | N/A | ✅ Connector cert rotation transparent to sessions | ❌ No hot-reload of certs; client must reconnect with new cert; SSH CA not reneweable without restart (documented limitation) |

### A.4 Session Management

| Feature | OpenZiti | Cloudflare Access | BeyondCorp | Zscaler ZPA | Our Project |
|---------|----------|-------------------|------------|-------------|-------------|
| **Session TTL enforcement** | ✅ API session TTL; posture check timeout terminates sessions | ✅ Per-app session duration (configurable: 30min–720h); forced re-auth | ✅ Per access-level session length; re-auth required | ✅ Configurable session timeouts per policy | ⚠️ Decision TTL advertised by CP (`ttl_seconds` in response); gateway config `decision_cache_ttl` exists (default 60s); but no mid-session re-auth enforcement |
| **Admin session kill** | ✅ Admin API to remove sessions/terminate identities | ✅ Dashboard: revoke all active sessions per user | ⚠️ Remove device from inventory; revoke Google session | ✅ Admin Portal: terminate user sessions | ⚠️ CRL-based revocation kills sessions of revoked certs; `DELETE /admin/device-certs/{serial}`; no per-user session kill for non-cert sessions |
| **Session telemetry** | ⚠️ Events API; Prometheus metrics | ✅ Extensive: audit logs, request logs, per-session details in dashboard, Logpush to SIEM | ✅ Cloud Logging; BigQuery export | ✅ Detailed session logs; integration with SIEM | ✅ `POST /api/v1/pep/sessions/start` and `/end`; `GET /api/v1/admin/sessions` with `decision_id`, `bytes_in/out`, `end_reason` |
| **Graceful session drain** | ✅ Edge router graceful shutdown drains active sessions | ⚠️ Cloudflared graceful shutdown with drain timeout | N/A | ✅ Connector graceful shutdown | ⚠️ `GracefulShutdown` stub exists in `gateway/internal/usecase/lifecycle/lifecycle.go`; TODO: drain sessions, flush metrics; currently skeleton only |

### A.5 Network Security & Micro-Segmentation

| Feature | OpenZiti | Cloudflare Access | BeyondCorp | Zscaler ZPA | Our Project |
|---------|----------|-------------------|------------|-------------|-------------|
| **L4 micro-segmentation** | ✅ Service-level isolation; each service is a dark service; dial policies control L4 access per identity | ✅ Per-app isolation; private network routing with WARP; origin-level rules | ⚠️ IAP provides app-level; VPC firewall rules for L4 | ✅ Application segments with port ranges; micro-segmentation by app | ✅ Network segmentation WAN/DMZ/LAN via Terraform; gateway routes per `resource_type:host:port`; iptables rules block direct WAN→LAN |
| **L7 inspection / policies** | ❌ Overlay is opaque at L7 (dark services model); no built-in L7 inspection | ✅ Full L7: HTTP policies, request headers, path-based rules, DLP inline, browser isolation | ✅ IAP is L7-native: per-resource HTTP policies, context-aware headers injected | ✅ Browser Access for L7 inspection; SSL inspection via ZIA | ❌ TCP proxy is L7-opaque; no HTTP-level inspection or policy |
| **SSRF protection** | ✅ Dark services model inherently prevents SSRF; only explicitly bound services reachable | ✅ WAF rules; metadata endpoint protection; loopback blocking default | ⚠️ Relies on GCP VPC firewall + metadata protection (v2 metadata endpoint) | ✅ App connector whitelists; no lateral movement by design | ⚠️ Comment in `tcp.go` mentions "no loopback address (127.0.0.1, ::1) except in lab"; no metadata endpoint blocking (169.254.169.254); no full SSRF mitigation |
| **Rate limiting** | ⚠️ Controller rate limits configurable; no built-in per-identity rate limit in overlay | ✅ Cloud-native rate limiting; per-IP, per-path, per-user composable rules | ✅ Cloud Load Balancing rate limits; per-identity quotas | ✅ Cloud-based rate limiting per policy | ✅ Per-IP token-bucket rate limiter on CP (`NewIPRateLimiter`); per-PEP rate limiter (`HandlerByPEP`); applied to public and PEP endpoints |

### A.6 Observability & Audit

| Feature | OpenZiti | Cloudflare Access | BeyondCorp | Zscaler ZPA | Our Project |
|---------|----------|-------------------|------------|-------------|-------------|
| **Structured audit logs** | ✅ Events API; JSON events for auth, session, policy changes | ✅ Logpush: JSON NDJSON to S3/GCS/Datadog/Splunk; per-request audit with all context | ✅ Cloud Audit Logs + Access Transparency; BigQuery export | ✅ LSS (Log Streaming Service): JSON to SIEM (Splunk, QRadar, etc.) | ✅ Audit in SQLite: `issue_ssh_cert`, `connect`, `create_policy`, `activate_policy` etc.; `GET /admin/audit?limit=50&action=connect`; JSON-formatted logs via `slog` |
| **SIEM integration** | ⚠️ Custom export required (REST events API) | ✅ Native: Logpush, Datadog, Splunk, Sumo Logic, S3, R2 | ✅ Cloud Logging → BigQuery/Pub-Sub → SIEM | ✅ Native LSS to any SIEM | ❌ No SIEM export; SQLite local storage only; webhook planned |
| **Distributed tracing** | ⚠️ Internal tracing; no OpenTelemetry native | ✅ Cloudflare Trace; cf-ray ID across requests | ✅ Cloud Trace integration | ⚠️ Transaction ID per session | ⚠️ `X-Request-ID` generation/propagation; `pep_id` in context; no OpenTelemetry (planned) |
| **Prometheus metrics** | ✅ Built-in `/metrics` endpoint | N/A (cloud metrics dashboard) | N/A (Cloud Monitoring) | N/A (cloud dashboard) | ❌ Planned (`/metrics` endpoint listed in roadmap) |

### A.7 Resilience & High Availability

| Feature | OpenZiti | Cloudflare Access | BeyondCorp | Zscaler ZPA | Our Project |
|---------|----------|-------------------|------------|-------------|-------------|
| **Controller/CP HA** | ✅ Controller HA via raft consensus (v0.31+); multiple controllers in cluster | ✅ Global anycast; hundreds of PoPs; inherently HA | ✅ Google's global infrastructure; multi-region IAP | ✅ Multi-DC cloud broker; active-active | ❌ Single instance only; no clustering; documented limitation |
| **Edge router / gateway failover** | ✅ Multiple edge routers per fabric; smart routing failover | ✅ Multiple cloudflared replicas; automatic failover | N/A | ✅ Multiple app connectors per group; automatic failover | ❌ Single gateway; no failover |
| **CP-down fallback** | ✅ Edge routers cache sessions; existing sessions persist; new sessions denied until controller recovers | ✅ Edge caches sessions; tunnel reconnect with backoff | N/A | ✅ Cached sessions continue; new ones denied | ⚠️ Gateway `decision_cache_ttl` (60s default) means cached decisions survive brief CP outages; heartbeat warns on CP unavailable; no explicit degraded-mode handling |
| **Decision caching** | ✅ Session state cached on edge routers | ✅ Worker cache at edge | N/A | ✅ Broker session cache | ✅ Configuration exists: `decision_cache_ttl` (default 60s), `decision_cache_max_entries` (default 5000); implementation in gateway config |

### A.8 Protocol & Connectivity

| Feature | OpenZiti | Cloudflare Access | BeyondCorp | Zscaler ZPA | Our Project |
|---------|----------|-------------------|------------|-------------|-------------|
| **Protocol versioning** | ⚠️ SDK versions; controller-edge compatibility matrix | N/A (cloud-managed) | N/A | N/A (cloud-managed) | ⚠️ `ProtocolVersion` field (currently v1) in CONNECT messages; negotiation logic has TODO stub; test skipped |
| **Supported protocols** | ✅ TCP, UDP, HTTP(S); any IP protocol via tunneler | ✅ HTTP/HTTPS (L7), TCP (Spectrum), UDP (WARP) | ✅ HTTP/HTTPS (IAP); TCP via IAP-TCP tunneling | ✅ TCP, UDP; browser-based RDP/SSH/VNC | ✅ SSH (via SSH certs) + mTLS TCP proxy; HTTP tunneled through TCP; no native UDP |
| **Device posture** | ✅ OS version, OS type, domain, MAC address, process check (e.g., antivirus running) | ✅ WARP client: disk encryption, OS version, firewall, specific file/process, serial number, domain join | ✅ Device attributes: OS version, disk encryption, screen lock, enterprise cert, last sync | ✅ Posture profiles: OS patch level, disk encryption, firewall, AV running, personal/corporate | ❌ TODO in `client/internal/infra/tunnel/protocol.go` line 75: "Définir le schéma des informations device (OS, version, posture)"; device info placeholder exists but not collected or evaluated |

---

## B. Top 15 Features an Expert Jury Would Test

An expert jury evaluating a ZTNA demo would systematically verify these capabilities, roughly in order of criticality:

### B.1 — Default-Deny Enforcement
**What they test**: Attempt access to a resource with no policy match. Expect: connection refused with audit trail.  
**Our status**: ✅ Default-deny is implemented and enforced in the policy engine.

### B.2 — mTLS Handshake & Certificate Validation
**What they test**: Present an invalid/expired/self-signed cert → expect rejection at TLS level. Present a valid cert → expect handshake success.  
**Our status**: ✅ Gateway enforces `RequireAndVerifyClientCert` with Device CA.

### B.3 — Certificate Revocation Under Load
**What they test**: Revoke a device cert, then attempt connection. Measure time from revocation to enforcement. Test with active session: does it get killed?  
**Our status**: ✅ CRL refresh + `KillRevoked` on gateway. Sub-minute enforcement (configurable, default 60s).

### B.4 — Policy Change Propagation
**What they test**: Change policy from allow to deny for an active user. Measure: how quickly is the new policy effective? Are existing sessions affected?  
**Our status**: ⚠️ New connections will use fresh policy if decision cache has expired. Existing sessions are NOT re-evaluated — they persist for the session lifetime. Gap: no push-based policy propagation; no mid-session re-authorization.

### B.5 — Session TTL Enforcement & Re-Authorization
**What they test**: Set a short session TTL (e.g., 5 min). Verify session is terminated or re-auth is required at expiry.  
**Our status**: ⚠️ Decision TTL is advertised by CP but not enforced as hard session timeout on the gateway. Sessions persist beyond TTL.

### B.6 — Admin Session Kill
**What they test**: From admin console, kill an active user session. Verify immediate disconnection.  
**Our status**: ⚠️ Possible via cert revocation (`DELETE /admin/device-certs/{serial}`), which triggers CRL refresh → kill. No per-session or per-user instant kill mechanism.

### B.7 — Audit Trail Completeness
**What they test**: Perform actions (login, connect, policy change, revocation), then query audit logs. Expect structured JSON with timestamp, actor, action, resource, decision, source IP.  
**Our status**: ✅ Comprehensive audit: `issue_ssh_cert`, `issue_device_cert`, `connect` (with `decision`, `reason`, `pep_id`, `src_ip`), `create_policy`, `activate_policy`. Queryable via REST.

### B.8 — Device Posture Assessment
**What they test**: Connect from a device with outdated OS / no disk encryption / no firewall. Expect: denied or downgraded access level.  
**Our status**: ❌ Not implemented. Device info placeholder exists in protocol but nothing is collected or evaluated.

### B.9 — Continuous Authorization
**What they test**: During an active session, change the user's group membership (e.g., remove from admin group). Expect: session terminated or access reduced.  
**Our status**: ❌ Not implemented. Authorization is only evaluated once at connection time.

### B.10 — Rate Limiting Under Attack
**What they test**: Flood the login/authorize endpoint. Expect: per-IP and per-user throttling with proper HTTP 429 responses.  
**Our status**: ✅ Per-IP token-bucket on public endpoints; per-PEP-ID on PEP endpoints.

### B.11 — Network Segmentation Integrity
**What they test**: From WAN, attempt to reach LAN directly (bypass gateway). Expect: no route, packet dropped.  
**Our status**: ✅ Terraform-managed iptables; WAN↔LAN isolation; only gateway bridges.

### B.12 — SSRF / Lateral Movement Prevention
**What they test**: Client sends CONNECT request targeting `127.0.0.1`, `169.254.169.254` (cloud metadata), or internal-only addresses. Expect: blocked.  
**Our status**: ⚠️ Loopback comment exists; no systematic block of metadata endpoints or internal ranges.

### B.13 — CP Unavailability Resilience
**What they test**: Kill the control plane. Verify: existing sessions continue; new sessions handled gracefully (cached decisions or clean rejection).  
**Our status**: ⚠️ Decision cache provides ~60s grace. Gateway heartbeat loop logs warnings. No explicit "degraded mode" with user-visible status.

### B.14 — Gateway Graceful Shutdown
**What they test**: Signal gateway for shutdown during active sessions. Expect: sessions drain before process exits; no abrupt disconnections.  
**Our status**: ⚠️ `GracefulShutdown` function exists but is a TODO stub. Active sessions would be killed on shutdown.

### B.15 — Protocol Versioning & Negotiation
**What they test**: Client sends a newer protocol version. Expect: server negotiates down gracefully or rejects with clear error.  
**Our status**: ⚠️ `ProtocolVersion` field exists in CONNECT; no negotiation logic implemented; test skipped.

---

## C. Minimum Viable Feature Set for "Industrial Grade"

To be classified as "industrial grade" by an expert jury, a ZTNA solution **MUST** demonstrate all items in Tier 1, and **SHOULD** demonstrate Tier 2.

### Tier 1 — MUST HAVE (Hard Requirements)

| # | Feature | Our Status | Gap |
|---|---------|------------|-----|
| 1 | **Default-deny policy enforcement** | ✅ Done | — |
| 2 | **mTLS for all data-plane traffic** | ✅ Done | — |
| 3 | **Certificate revocation with active session kill** | ✅ Done | CRL-based, sub-minute latency |
| 4 | **Structured audit trail (all auth/authz events)** | ✅ Done | SIEM export missing |
| 5 | **Session TTL enforcement** | ⚠️ Partial | Decision TTL exists but not enforced as hard session timeout |
| 6 | **Rate limiting on control plane** | ✅ Done | — |
| 7 | **Network segmentation (no bypass possible)** | ✅ Done | — |
| 8 | **Short-lived credentials** | ✅ Done | SSH certs (min 60s), device certs with TTL |
| 9 | **Identity via external IdP (OIDC/SAML)** | ✅ Done | — |
| 10 | **SSRF protection (block loopback/metadata)** | ⚠️ Partial | Loopback noted but not systematically blocked |

### Tier 2 — SHOULD HAVE (Expected in a Demo)

| # | Feature | Our Status | Gap |
|---|---------|------------|-----|
| 11 | **Continuous authorization (periodic re-eval)** | ❌ Missing | Critical gap |
| 12 | **Device posture assessment** | ❌ Missing | Protocol placeholder exists |
| 13 | **Graceful session drain on shutdown** | ⚠️ Stub | TODO in lifecycle.go |
| 14 | **Admin instant session kill** | ⚠️ Indirect | Only via cert revocation |
| 15 | **CP-down fallback mode** | ⚠️ Partial | Cache-based, no explicit mode |
| 16 | **SIEM-compatible log export** | ❌ Missing | Webhook planned |
| 17 | **mTLS cert rotation without drop** | ❌ Missing | Restart required |
| 18 | **Protocol version negotiation** | ⚠️ Stub | Field exists, no logic |
| 19 | **Controller HA / clustering** | ❌ Missing | Single instance |
| 20 | **Prometheus / OpenTelemetry metrics** | ❌ Missing | Planned |

### Scoring Summary

- **Tier 1 (10 hard requirements)**: 7 fully met, 2 partial, 1 partial → **~80% compliant**
- **Tier 2 (10 advanced features)**: 0 fully met, 5 partial/stub, 5 missing → **~25% compliant**
- **Overall industrial-grade readiness**: **Not yet industrial grade.** The project has solid fundamentals but lacks continuous authorization, device posture, and operational resilience features.

---

## D. Implementation Recommendations

> **Note**: These are concrete, code-level design recommendations for Go. No code is being written or modified.

---

### D.1 — Continuous Authorization (Critical Gap)

**What to build**: A goroutine per active session that periodically re-evaluates the authorization decision.

**Architecture**:
```
Session Start → spawn reauth goroutine
  └─ every N seconds (configurable, e.g., 30s–300s):
       call CP /api/v1/pep/authorize with same subject/resource
       if decision changed to "deny":
           close session connection (graceful drain with 5s deadline)
           report session.end with end_reason="reauthorization_denied"
```

**Where in the codebase**:
- New file: `gateway/internal/infra/session/reauth.go`
- Integration point: `gateway/internal/usecase/connect/handler.go` at line ~140 (after successful authorization, before proxy start)
- Config: Add `reauth_interval` to gateway config struct in `gateway/internal/config/config.go`

**Key design decisions**:
- Use `context.WithCancel` — the reauth goroutine gets a cancel func; on deny, it cancels the parent context, which will interrupt `io.Copy` in the TCP proxy
- Rate-limit reauth calls: jitter the interval ±20% to avoid thundering herd against CP
- On CP unavailable during reauth: **do NOT terminate** — last cached decision holds until cache TTL expires (fail-open for availability, configurable to fail-closed)

**Go sketch**:
```go
type ReauthLoop struct {
    interval   time.Duration
    jitter     float64
    authzClient *authorize.Client
    req         *authorize.AuthzRequest
    cancel      context.CancelFunc
    log         *slog.Logger
}

func (r *ReauthLoop) Run(ctx context.Context) {
    ticker := time.NewTicker(r.jitteredInterval())
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            decision, err := r.authzClient.Authorize(r.req)
            if err != nil {
                r.log.Warn("reauth CP unreachable, keeping session", "error", err)
                continue
            }
            if decision.Effect != "allow" {
                r.log.Info("reauth denied, terminating session",
                    "reason", decision.Reason)
                r.cancel() // cascades to proxy
                return
            }
            ticker.Reset(r.jitteredInterval())
        }
    }
}
```

---

### D.2 — Device Posture Assessment

**What to build**: Client collects device state, sends it in CONNECT request; gateway/CP evaluates it against policy conditions.

**Phase 1 — Collection (client-side)**:
- Extend `ConnectRequest` (already has `DeviceInfo` placeholder at `client/internal/infra/tunnel/protocol.go` line 75)
- Collect: OS name, OS version, hostname, disk encryption status, firewall status
- On Linux: parse `/etc/os-release`, check `dmsetup status` or `lsblk` for LUKS, check `ufw status` or `iptables -L`
- On macOS: `sw_vers`, `fdesetup status`, `defaults read /Library/Preferences/com.apple.alf globalstate`

**Phase 2 — Evaluation (CP-side)**:
- Add `device_posture` conditions to policy rules in `control-plane/internal/service/policy/service.go`
- New rule field: `device_requirements: { min_os_version: "22.04", disk_encryption: required, firewall: required }`
- evaluation: if `device_requirements` is set, compare against `context.device_info` from the authorize request

**Structs**:
```go
// In connect request / authorize request
type DeviceInfo struct {
    OS              string `json:"os"`              // "linux", "darwin", "windows"
    OSVersion       string `json:"os_version"`      // "22.04"
    Hostname        string `json:"hostname"`
    DiskEncrypted   bool   `json:"disk_encrypted"`
    FirewallEnabled bool   `json:"firewall_enabled"`
    AVRunning       bool   `json:"av_running"`
    // Future: serial number, MDM enrollment, etc.
}

// In policy rule
type DeviceRequirements struct {
    MinOSVersion    string `json:"min_os_version,omitempty"`
    DiskEncryption  string `json:"disk_encryption,omitempty"`  // "required"|"optional"
    Firewall        string `json:"firewall,omitempty"`         // "required"|"optional"
}
```

---

### D.3 — Session TTL Hard Enforcement

**What to build**: Gateway enforces `ttl_seconds` from the authorization decision as a hard session deadline.

**Where**:
- In `gateway/internal/usecase/connect/handler.go`, after line ~140 (successful authorization):
  - Create context: `ctx, cancel := context.WithTimeout(ctx, time.Duration(decision.TTL) * time.Second)`
  - Pass this context into `proxy.Relay(ctx, ...)` — when context expires, both sides of the proxy get closed
  - Report `session.end` with `end_reason="ttl_expired"`

**Key consideration**: The TCP proxy (`gateway/internal/infra/proxy/tcp.go`) already supports `context.Context` cancellation (its `Relay` method uses `ctx`). The main change is wrapping the context with a deadline derived from the CP's TTL.

---

### D.4 — SSRF Protection

**What to build**: Validate target host/IP in CONNECT request before opening proxy connection.

**Where**: `gateway/internal/infra/proxy/tcp.go`, before `net.Dial` (or in the handler validation step at `handler.go` line ~92).

**Blocked ranges**:
```go
var blockedCIDRs = []string{
    "127.0.0.0/8",       // Loopback
    "::1/128",           // IPv6 loopback
    "169.254.169.254/32",// Cloud metadata (AWS, GCP, Azure)
    "100.100.100.200/32",// Alibaba Cloud metadata
    "fd00:ec2::254/128", // AWS IPv6 metadata
    "10.0.0.0/8",        // Optionally block if not in allowed routes
    "172.16.0.0/12",     // Configurable
    "192.168.0.0/16",    // Configurable
}
```

**Logic**: Resolve the hostname to IP, check against blocked CIDR list and an explicit allow-list from config (gateway routes). Reject with audit log entry if blocked.

**Critical**: DNS resolution must happen on the gateway side, and the resolved IP must be validated — not just the hostname. This prevents DNS rebinding attacks.

---

### D.5 — Graceful Session Drain

**What to build**: On SIGTERM/SIGINT, stop accepting new connections, wait for active sessions to complete (with deadline), then exit.

**Where**: `gateway/internal/usecase/lifecycle/lifecycle.go` (currently TODO stub) and `gateway/internal/bootstrap/app.go` line 87.

**Design**:
```go
func GracefulShutdown(ctx context.Context, listener net.Listener, sessions *session.Manager, drainTimeout time.Duration) error {
    // 1. Stop accepting new connections
    listener.Close()
    
    // 2. Wait for active sessions with deadline
    drainCtx, cancel := context.WithTimeout(ctx, drainTimeout)
    defer cancel()
    
    // 3. Poll session count
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-drainCtx.Done():
            // Force-close remaining sessions
            sessions.CloseAll("shutdown_timeout")
            return nil
        case <-ticker.C:
            if sessions.ActiveCount() == 0 {
                return nil
            }
        }
    }
}
```

- `session.Manager` already exists — add `ActiveCount()` and `CloseAll(reason)` methods
- Config: `graceful_drain_timeout: 30s` in gateway config

---

### D.6 — Admin Session Kill (Instant)

**What to build**: CP endpoint to kill specific sessions; gateway receives kill command via push or poll.

**Option A (Pull-based, simpler)**: Gateway heartbeat response includes a `kill_sessions` field with serial numbers or decision IDs to terminate. Gateway processes this on each heartbeat.

**Option B (Push-based, more robust)**: WebSocket or long-poll channel from gateway to CP for real-time commands.

**Recommendation**: Start with Option A (aligns with existing heartbeat). Add to heartbeat response:

```go
// CP heartbeat response (extend existing)
type HeartbeatResponse struct {
    Status       string   `json:"status"`       // "registered"
    KillSessions []string `json:"kill_sessions"` // decision_ids to terminate
    PolicyVersion int     `json:"policy_version"` // for staleness detection
}
```

Gateway side: on each heartbeat response, iterate `kill_sessions`, look up in `session.Manager`, close matching sessions.

---

### D.7 — mTLS Certificate Rotation Without Drop

**What to build**: Hot-reload TLS config when cert files change, without dropping existing connections.

**Where**: Gateway TLS listener setup (in bootstrap).

**Go approach**: Use `tls.Config.GetCertificate` or `tls.Config.GetConfigForClient` callbacks instead of static cert loading:

```go
type CertReloader struct {
    certPath string
    keyPath  string
    mu       sync.RWMutex
    cert     *tls.Certificate
}

func (cr *CertReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
    cr.mu.RLock()
    defer cr.mu.RUnlock()
    return cr.cert, nil
}

func (cr *CertReloader) Watch(ctx context.Context) {
    // Use fsnotify or poll interval to detect changes
    // On change: load new cert, swap under write lock
}
```

Existing connections keep their TLS session; new connections use the updated cert. This is a standard Go pattern.

For the CA cert (used to verify client certs), use `tls.Config.VerifyPeerCertificate` with a dynamically-loaded CA pool.

---

### D.8 — SIEM-Compatible Log Export

**What to build**: Webhook or streaming output for audit events.

**Options** (ranked by effort):
1. **Stdout structured JSON** (minimal effort): Already using `slog` with JSON format. Redirect stdout to Fluentd/Filebeat/Vector → SIEM. This works today with log aggregation.
2. **Webhook** (medium effort): New goroutine that reads from an in-memory audit queue and POSTs to a configurable URL, with retries and circuit breaker.
3. **OpenTelemetry Logs** (higher effort, future-proof): Emit audit events as OTel log records via OTLP gRPC.

**Recommendation**: Option 1 is immediate (document it). Option 2 as next step:

```go
// control-plane/internal/service/audit/webhook.go
type WebhookExporter struct {
    url    string
    client *http.Client
    queue  chan AuditEvent
}

func (w *WebhookExporter) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case event := <-w.queue:
            // POST with retries, exponential backoff
        }
    }
}
```

---

### D.9 — CP High Availability

**What to build**: Multi-instance control plane with shared state.

**Path to HA**:
1. **Replace SQLite with PostgreSQL** for the store layer (the `port.Store` interface already abstracts storage — add a `postgres` implementation alongside `sqlite`)
2. **Stateless CP instances**: Since auth is JWT-based and policy is read from DB, multiple CP instances can serve requests independently behind a load balancer
3. **Leader election** (for CRL signing, SSH CA key management): Use Kubernetes leader election or etcd for single-writer operations
4. **JWKS cache**: Replace in-memory JWKS cache with Redis or distributed cache

**Simplest first step**: Move to PostgreSQL + 2 CP instances behind an HAProxy or Kubernetes Service. Most of the code works as-is because the domain layer uses interfaces.

---

### D.10 — Decision Caching with Explicit TTL

**What to build**: The gateway config already has `decision_cache_ttl` and `decision_cache_max_entries`. Ensure the cache is actually implemented and used in the authorize flow.

**Where**: `gateway/internal/infra/authz/` — before calling CP, check cache; on CP response, store with TTL.

**Design**:
```go
type DecisionCache struct {
    mu      sync.RWMutex
    entries map[string]*cachedDecision // key = hash(subject+action+resource)
    ttl     time.Duration
    maxKeys int
}

type cachedDecision struct {
    decision *AuthzDecision
    expiry   time.Time
}
```

**Eviction**: LRU or time-based sweep every 10s. On CRL update or policy version change detected via heartbeat: **flush the entire cache** to ensure fresh decisions.

---

## Summary of Priority Actions

| Priority | Action | Effort | Impact |
|----------|--------|--------|--------|
| 🔴 P0 | Implement continuous re-authorization (D.1) | 2-3 days | Closes biggest gap vs every competitor |
| 🔴 P0 | Enforce session TTL as hard deadline (D.3) | 0.5 day | Trivial change, high demo value |
| 🔴 P0 | SSRF protection — block dangerous IPs (D.4) | 0.5 day | Security table-stakes |
| 🟠 P1 | Device posture collection + evaluation (D.2) | 3-5 days | Expected in any ZTNA demo |
| 🟠 P1 | Graceful session drain (D.5) | 1 day | Production-readiness signal |
| 🟠 P1 | Admin session kill via heartbeat (D.6) | 1-2 days | High demo value |
| 🟡 P2 | mTLS cert hot-reload (D.7) | 1 day | Operational maturity |
| 🟡 P2 | SIEM webhook export (D.8) | 1-2 days | Enterprise requirement |
| 🟡 P2 | Protocol version negotiation (complete stub) | 0.5 day | Completeness |
| ⚪ P3 | CP HA / PostgreSQL migration (D.9) | 1-2 weeks | Day-2 operations |
| ⚪ P3 | Prometheus + OpenTelemetry (D.10) | 2-3 days | Observability |

---

*This document is a point-in-time technical assessment. It references public documentation for OpenZiti, Cloudflare, Google BeyondCorp, and Zscaler ZPA as of early 2026.*
