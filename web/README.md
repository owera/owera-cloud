# Owera Agentic — web/

The customer-facing dashboard at `app.owera.ai`. Next.js 15 App Router, React
19, TypeScript strict, Tailwind v4.

This is the scaffold. Real auth, real Stripe portal, and live API calls all
arrive in follow-on PRs.

## Quick start

```bash
cd web
npm install
npm run dev        # http://localhost:3000
```

> Package manager: **npm**. The repo doesn't ship a lockfile for any other
> manager — if you prefer pnpm/yarn, do that in a separate PR so we don't end
> up with two lockfiles in tree.

## Environment

Copy `.env.example` to `.env.local` and adjust:

| Var                          | Default                  | Notes                                              |
| ---------------------------- | ------------------------ | -------------------------------------------------- |
| `NEXT_PUBLIC_API_URL`        | `http://localhost:8080`  | Where `lib/api-client.ts` and the proxy send calls |
| `NEXT_PUBLIC_AUTH_PROVIDER`  | `mock`                   | `mock` \| `clerk` \| `workos` (only mock is wired) |

In **mock mode** (the default), `lib/auth.ts` returns a fake signed-in user
(`dev@owera.ai`, tenant `tnt_mock_0001`) and the dashboard renders fixture
data when the upstream API is unreachable. Look for the small orange
"FIXTURE DATA" badge in the corner of each page.

## Verify

```bash
npm install
npx tsc --noEmit       # types check
npx next lint          # eslint
npx next build         # production build
```

If `next build` warns about a peer-dep mismatch on `react@19.0.0-rc-…`, that's
expected — Next 15 ships an RC of React 19; pin both together when bumping.

## Architecture notes

- **All API calls flow through `lib/api-client.ts`.** Nothing else reads
  `NEXT_PUBLIC_API_URL` or hardcodes an absolute URL.
- **Types live in `lib/types.ts`** with a `// SYNC: api/openapi.yaml` banner.
  Until the API spec stabilises, this file is hand-edited; afterwards swap in
  a generated client (see `.gitignore`: `web/lib/api/generated/`).
- **Auth is stubbed** in `lib/auth.ts`. The interface is real; the
  implementation just returns a fake user. Wire Clerk or WorkOS in a separate
  PR so we can keep this one reviewable.
- **No shadcn-ui dependency.** We own the radix-powered primitives under
  `components/ui/*` directly. Add new primitives there.
- **Design system:** dense, monospace, table-heavy. Linear/Vercel/Stripe-
  console energy. Single accent (Owera primary, `#5b8def`). No gradients, no
  drop shadows on cards (`border` + `bg-card` only). Job states use a fixed
  palette defined in `styles/globals.css` and consumed via
  `components/job-status-badge.tsx`.

## Layout

```
web/
├── app/
│   ├── layout.tsx            root layout — dark theme by default
│   ├── page.tsx              redirect: signed-in → /dashboard, else marketing
│   ├── (marketing)/page.tsx  splash for signed-out users
│   ├── (dashboard)/
│   │   ├── layout.tsx        sidebar chrome + AuthGuard wrap
│   │   ├── dashboard/        overview: usage, recent jobs, cost-to-date
│   │   ├── jobs/             list + [id] detail (state, ledger, outputs)
│   │   ├── billing/          Stripe Customer Portal stub
│   │   ├── api-keys/         CRUD over API keys
│   │   └── support/          docs links + ticket inbox stub
│   └── api/proxy/[...path]/  server-side proxy to api.owera.ai
├── components/               domain components (jobs table, status badge, …)
├── components/ui/            owned radix primitives (button, card, table, …)
├── lib/                      api-client, auth stub, types, format helpers
├── styles/globals.css        Tailwind v4 + Owera design tokens
└── public/favicon.svg        single-color favicon stub
```

## What the api/ agent needs to match

Hand-written types in `lib/types.ts` and call sites in `lib/api-client.ts`
together describe the surface this UI expects. Specifically:

- `GET /v1/jobs?limit=&state=` → `Job[]`
- `GET /v1/jobs/:id` → `Job`
- `GET /v1/jobs/:id/ledger` → `JobLedgerEntry[]`
- `GET /v1/skus` → `SKU[]`
- `GET /v1/usage/current` → `UsageMeter`
- `GET /v1/api-keys` → `ApiKey[]`
- `POST /v1/api-keys { name, scopes }` → `ApiKey & { secret }`
- `DELETE /v1/api-keys/:id` → `204`
- `POST /v1/billing/portal` → `{ url }` (referenced in the billing page copy)
- `GET /v1/support/tickets`, `POST /v1/support/tickets` (referenced in the support page copy)

Job states are exactly: `submitted | queued | running | succeeded | failed | cancelled`.

Errors use:

```jsonc
{ "code": "<machine-readable>", "message": "<human-readable>", "requestId": "..." }
```
