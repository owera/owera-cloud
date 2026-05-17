# Secrets manifest

Authoritative catalogue of every secret the Owera cloud plane consumes. **No values appear in this file.** Each secret is identified by name, store, owner, and rotation cadence.

Operators reconcile this manifest against the live secret stores quarterly. Any addition or removal of a secret is a PR-reviewed change to this file.

## Conventions

- **Store** — where the canonical value lives. Never check secrets into git.
  - `fly` — `fly secrets set` on the `owera-agentic-api` app.
  - `vercel` — Project Settings → Environment Variables (Production / Preview).
  - `1password` — 1Password vault `Owera / Cloud Plane`.
  - `keychain` — macOS Keychain on the Mac mini gateway (owner: `claw3`).
  - `gateway-fs` — file on the gateway filesystem, `chmod 600`, owned by the cloudflared service account.
- **Owner** — the human who can rotate the secret without escalation. CISO can rotate any.
- **Rotation** — calendar cadence. `incident` means rotate only on suspected compromise.
- **Status** — `[x]` provisioned; `[ ]` TODO; `[!]` rotation overdue.

## Cloud plane API (Fly)

| Status | Name | Store | Owner | Rotation | Notes |
|--------|------|-------|-------|----------|-------|
| `[ ]` | `STRIPE_SECRET_KEY` | `fly`, `1password` | CFO | 180d | Live keys only in fly; test keys in 1password for staging |
| `[ ]` | `STRIPE_WEBHOOK_SECRET` | `fly` | CFO | 180d | Rotated together with `STRIPE_SECRET_KEY` |
| `[ ]` | `STRIPE_RESTRICTED_KEY_REPORTING` | `fly` | CFO | 180d | Read-only key for the billing/reporting pipeline |
| `[ ]` | `CLOUDFLARE_API_TOKEN` | `fly`, `1password` | SRE | 90d | Scoped to Zone:DNS:Edit on owera.ai zone only |
| `[ ]` | `CLERK_SECRET_KEY` | `fly` | SRE | 180d | TL pinned Clerk over WorkOS for Wave-8 (WS-15); consumed by api JWT-verify path |
| `[ ]` | `CLERK_PUBLISHABLE_KEY` | `vercel` | SRE | 180d | Public; rotated alongside the secret key |
| `[ ]` | `CLERK_JWT_TEMPLATE_OWERA_API` | `fly` | SRE | n/a | Name of the Clerk JWT template the api recognises ("owera-api"); referenced by web/lib/auth.ts getApiToken() |
| `[ ]` | `OPERATOR_PLANE_PUBKEY_ED25519` | `fly` | CISO | incident | The operator-plane minisign pubkey; api verifies JWS payloads with it |
| `[ ]` | `SQLITE_ENCRYPTION_KEY` | `fly` | CISO | 365d | sqlcipher passphrase for the api-local cache; rotation requires re-encrypt migration |
| `[ ]` | `JWT_SIGNING_KEY` | `fly` | SRE | 90d | HS512; rotated with overlap window (api accepts old + new for 24h) |
| `[ ]` | `SES_SMTP_USERNAME` | `fly` | SRE | 180d | Transactional email |
| `[ ]` | `SES_SMTP_PASSWORD` | `fly` | SRE | 180d | Rotated with `SES_SMTP_USERNAME` |
| `[ ]` | `SENTRY_DSN` | `fly`, `vercel` | SRE | incident | Error reporting; rotation requires Sentry project re-key |

## Cloud plane web (Vercel)

| Status | Name | Store | Owner | Rotation | Notes |
|--------|------|-------|-------|----------|-------|
| `[ ]` | `NEXT_PUBLIC_API_URL` | `vercel` | SRE | n/a | `https://api.owera.ai`; not a secret but version-controlled in this manifest |
| `[ ]` | `NEXT_PUBLIC_STATUS_URL` | `vercel` | SRE | n/a | `https://status.owera.ai` |
| `[ ]` | `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` | `vercel` | SRE | 180d | Mirrors the api-tier value |
| `[ ]` | `CLERK_SECRET_KEY` | `vercel` | SRE | 180d | Only for SSR route handlers; never shipped to client |

## Operator-plane tunnel (Mac mini gateway)

| Status | Name | Store | Owner | Rotation | Notes |
|--------|------|-------|-------|----------|-------|
| `[ ]` | `CLOUDFLARED_TUNNEL_UUID` | `1password`, `gateway-fs` | SRE | incident | The tunnel id; non-secret but tracked |
| `[ ]` | `CLOUDFLARED_TUNNEL_CREDENTIALS` | `gateway-fs` | SRE | 365d | `/usr/local/etc/cloudflared/<UUID>.json`, chmod 600 |
| `[ ]` | `OPERATOR_PLANE_PRIVKEY_ED25519` | `keychain` | CISO | 365d | minisign private key; corresponds to `OPERATOR_PLANE_PUBKEY_ED25519` |

## CI / build

| Status | Name | Store | Owner | Rotation | Notes |
|--------|------|-------|-------|----------|-------|
| `[ ]` | `GH_DEPLOY_TOKEN_FLY` | `1password` | SRE | 90d | GitHub Actions → `fly deploy`; scoped to the api app |
| `[ ]` | `GH_DEPLOY_TOKEN_VERCEL` | `1password` | SRE | 90d | GitHub Actions → Vercel deploy hook |
| `[ ]` | `GITLEAKS_LICENSE` | `1password` | SRE | 365d | If we move to gitleaks Pro; OSS works without |

## Rotation procedure (summary)

1. Generate the new value in the source-of-truth system (Stripe dashboard, Cloudflare API tokens UI, etc.).
2. Set it in the store with a `_NEXT` suffix where the application supports overlap (JWT key, Stripe webhook secret with secondary slot).
3. Deploy the application; verify both old and new values are accepted.
4. Cut over: set the primary slot to the new value, retire the old.
5. Update the `Status` column in this manifest with the rotation date in the commit message.
6. For overlap-incapable secrets (`SQLITE_ENCRYPTION_KEY`, etc.), follow the per-secret playbook in `compliance/runbooks/`.

Full Sev1 procedure for a suspected key compromise lives in `../compliance/runbooks/incident-response.md`.
