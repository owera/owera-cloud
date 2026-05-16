# compliance/

Owera's compliance posture as **versionable, signable documents**. This directory is the source of truth for:

- **Policies** (`policies/`) — what we commit to do.
- **Runbooks** (`runbooks/`) — how we actually do it when something happens.
- **Audit controls** (`audit-controls/`) — the machine-readable mapping from frameworks (SOC 2, LGPD, GDPR) to evidence locations.

## Compliance roadmap

| Framework | Status | Target | Owner | Notes |
|-----------|--------|--------|-------|-------|
| **LGPD** (Brazil, Lei 13.709/2018) | In progress | Day-one (GA) | CISO | Required by jurisdiction — Owera Software Ltda is São Paulo-headquartered, customers are BR primary. Policies in this directory are LGPD-aligned out of the gate. |
| **SOC 2 Type 1** | Not started | GA + 12 months | CISO + external auditor | Common Criteria mapped in `audit-controls/soc2-cc.yaml`. Audit window: 1 day point-in-time. |
| **SOC 2 Type 2** | Not started | GA + 24 months | CISO + external auditor | 6-month observation window after Type 1 is clean. |
| **GDPR** | Conditional | First EU customer | CISO + DPO | Triggered on first EU customer signup. LGPD is intentionally closer to GDPR than to other regimes, so the lift is incremental. |
| **HIPAA** | Out of scope | n/a | n/a | Owera does not store PHI. If a healthcare customer requires it, this is a separate engagement. |
| **ISO 27001** | Deferred | TBD | CISO | Re-evaluate after SOC 2 Type 2. |

## How the docs hang together

```
compliance/
├── policies/          What we commit to
│   ├── security-policy.md
│   ├── access-control-policy.md
│   ├── incident-response-policy.md
│   ├── data-retention-policy.md
│   ├── change-management-policy.md
│   └── vendor-management-policy.md
├── runbooks/          How we execute the commitments
│   ├── on-call-runbook.md
│   ├── incident-response.md
│   ├── customer-data-export.md
│   └── customer-data-deletion.md
└── audit-controls/    The evidence layer
    ├── README.md
    └── soc2-cc.yaml
```

Each policy ends with a **version block** — a markdown table tracking author, date, and changes. Policies are reviewed annually (calendar Q1) and on any material change to the cloud-plane architecture.

## LGPD specifics

The LGPD-derived obligations baked into our policies:

| LGPD Article | Topic | Where it appears |
|---|---|---|
| Art. 8 | Consent (free, informed, unequivocal) | `policies/security-policy.md` §6, runbook `customer-data-export.md` |
| Art. 14 | Treatment of minors' data — explicit, specific, prominent parental consent for children under 12 | `policies/data-retention-policy.md` §4 (age-of-consent gate) |
| Art. 18 | Data subject rights — access, correction, portability, deletion | `runbooks/customer-data-export.md`, `runbooks/customer-data-deletion.md` |
| Art. 19 | Right of access / portability (machine-readable format) | `runbooks/customer-data-export.md` |
| Art. 48 | Breach notification — ANPD within "reasonable time" (treated as ≤72h, mirroring GDPR) | `runbooks/incident-response.md` §Communications |

The Brazilian DPO contact and ANPD reporting endpoint live in `runbooks/incident-response.md`.

## Owning each control

The RACI for who owns each policy / runbook is at the bottom of each document under "Ownership". The CISO is the accountable party for the compliance program as a whole.
