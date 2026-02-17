**FEATURES**

- **Configuration**: lecture et validation des paramètres OIDC, PEP, SSHCA, TLS et base de données. Voir [control-plane/internal/config/config.go](control-plane/internal/config/config.go).
- **Initialisation & wiring**: ouverture du store SQLite, génération/chargement de la clé SSH‑CA, seed des politiques et construction des services/middlewares. Voir [control-plane/internal/app/app.go](control-plane/internal/app/app.go).
- **API HTTP**: routes publiques, endpoints admin, endpoint de santé et prise en charge optionnelle d’un serveur PEP séparé (mTLS). Voir [control-plane/internal/api/httpserver/server.go](control-plane/internal/api/httpserver/server.go).
- **Authentification OIDC**: validation offline des JWT via JWKS (RS256 uniquement), mapping des claims (`sub`, `username`, `groups`). Voir [control-plane/internal/api/middleware/user_auth.go](control-plane/internal/api/middleware/user_auth.go) et [control-plane/internal/security/oidc/validator.go](control-plane/internal/security/oidc/validator.go).
- **Autorisation PEP**: mode `token` (X-PEP-ID + X-PEP-TOKEN) et mode `mtls` (vérification certificat client → PepID). Voir [control-plane/internal/api/middleware/pep_auth.go](control-plane/internal/api/middleware/pep_auth.go).
- **Gestion des politiques**: création de versions, activation, évaluation rules→allow/deny (default deny) et seed automatique. Voir [control-plane/internal/service/policy/service.go](control-plane/internal/service/policy/service.go) et [control-plane/internal/api/handlers/admin_policies.go](control-plane/internal/api/handlers/admin_policies.go).
- **Moteur de décision (PEP)**: endpoint d’autorisation qui évalue et renvoie la décision + raison. Voir [control-plane/internal/service/decision/service.go](control-plane/internal/service/decision/service.go) et [control-plane/internal/api/handlers/pep_authorize.go](control-plane/internal/api/handlers/pep_authorize.go).
- **Émission de certificats SSH**: SSH CA (génération/chargement ED25519), signature de certificats utilisateurs, résolution des principals (`${username}`, `${sub}`) et validation des TTL (min/default/max). Voir [control-plane/internal/crypto/sshca/sshca.go](control-plane/internal/crypto/sshca/sshca.go) et [control-plane/internal/service/credentials/service.go](control-plane/internal/service/credentials/service.go).
- **Audit**: enregistrement d’événements d’audit (issue cert, authorize, ops admin) dans la BD et endpoints de consultation. Voir [control-plane/internal/service/audit/service.go](control-plane/internal/service/audit/service.go) et [control-plane/internal/api/handlers/admin_audit.go](control-plane/internal/api/handlers/admin_audit.go).
- **Extraction IP source**: extraction `X-Forwarded-For`, `X-Real-IP`, RemoteAddr et lecture de `src_ip` depuis le contexte PEP pour remplir les entrées d’audit. Voir [control-plane/internal/api/handlers/audit_helpers.go](control-plane/internal/api/handlers/audit_helpers.go).
- **Tracing léger**: génération/propage de `X-Request-ID` et stockage du `pep_id` dans le contexte pour logging/audit. Voir [control-plane/internal/api/middleware/request_id.go](control-plane/internal/api/middleware/request_id.go) et [control-plane/internal/logger/logger.go](control-plane/internal/logger/logger.go).
- **TLS / mTLS serveur**: construction de la configuration TLS pour serveur public et serveur PEP (RequireAndVerifyClientCert si activé). Voir [control-plane/internal/api/httpserver/server.go](control-plane/internal/api/httpserver/server.go).
- **Stockage**: utilisation d’un store SQLite pour policies, users et audit (migrations + ouverture via `sqlite.Open`). Voir [control-plane/internal/app/app.go](control-plane/internal/app/app.go).

Notes opérationnelles :
- Les secrets (Keycloak client secrets, clés CA, etc.) sont actuellement configurés via fichiers/paramètres locaux — prévoir migration vers un coffre sécurisé avant production.
- Pour les tests locaux, les scripts de lab et d’E2E se trouvent dans `scripts/` et dans `lab/terraform/`.

Souhaitez‑vous que j’ajoute des exemples d’appels API et un script `scripts/validate-token.sh` ici ?
