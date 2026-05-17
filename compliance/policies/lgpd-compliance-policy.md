# LGPD compliance policy

**Scope:** all personal data of Brazilian data subjects processed by Owera Software Ltda in the course of operating Owera Agentic — customer payloads, account metadata, billing records, audit logs, employee records, and any third-party data ingested by agent jobs.

**Statutory basis:** Lei Geral de Proteção de Dados Pessoais (LGPD), Lei nº 13.709/2018, as amended.

**Authoritative regulator:** Autoridade Nacional de Proteção de Dados (ANPD).

This document is the LGPD-specific complement to [`security-policy.md`](./security-policy.md), [`access-control-policy.md`](./access-control-policy.md), and [`data-retention-policy.md`](./data-retention-policy.md). Where another policy and this one overlap, the stricter requirement controls.

## 1. Roles (Art. 5 definitions)

| LGPD term | Owera entity | Notes |
|-----------|--------------|-------|
| **Controller** (*controlador*, Art. 5 §VI) | **Owera Software Ltda** — CNPJ TBD, headquartered Macapá, Amapá, Brazil | Determines the purposes and essential means of treating customer and personnel personal data |
| **Operator / Processor** (*operador*, Art. 5 §VII) | Hosted infrastructure vendors (Fly.io, Vercel, Cloudflare, Clerk/WorkOS, Stripe, AWS, SES) | Treat personal data on Owera's behalf per documented contractual instructions; see [`vendor-management-policy.md`](./vendor-management-policy.md) |
| **Data subject** (*titular*, Art. 5 §V) | Any natural person whose personal data Owera treats — customers, prospects, employees, contractors, end-users of customer integrations | Holds all rights enumerated in Art. 18 |
| **Sensitive personal data** (*dado pessoal sensível*, Art. 5 §II) | Data revealing racial/ethnic origin, religious beliefs, political opinion, union membership, health, sex life, or genetic/biometric data | Owera **does not knowingly collect** sensitive data; the signup flow blocks profile fields for these categories. Agent jobs that pass sensitive data through payloads inherit Restricted classification per [`security-policy.md`](./security-policy.md) §2 |
| **Anonymized data** (*dado anonimizado*, Art. 5 §III, Art. 12) | Personal data subjected to reasonable technical means rendering re-identification impractical | Anonymized data falls outside LGPD scope; the audit-log tokenization path (see §5) produces anonymized references for retained chains |
| **Data Protection Officer** (*encarregado*, Art. 41) | See §3 below | Mandatory contact between controller, data subjects, and ANPD |

## 2. Legal bases for processing (Art. 7)

Every Owera treatment of personal data is grounded in one of the LGPD Art. 7 legal bases. The mapping is:

| Art. 7 basis | Owera treatments |
|--------------|------------------|
| **§I Consent** — free, informed, unequivocal, for a specific purpose (Art. 8) | Marketing emails; product analytics beyond aggregate; voluntary case-study participation; cross-border transfers in tenants without standard contract clauses |
| **§V Performance of a contract** | Account provisioning; billing; service delivery (running agent jobs on customer payloads); customer support |
| **§II Compliance with legal or regulatory obligation** | Tax records (Receita Federal — 5 years); SOC 2 / LGPD audit logs (7 years); ANPD breach notifications; court-ordered preservation |
| **§IX Legitimate interest** (with proportionality assessment) | Fraud prevention (rate-limiting, abuse detection); product analytics in aggregate; internal security monitoring; this basis is **never used for sensitive data** |
| **§IV Studies by research organization** | Not invoked — Owera is not a research organization |
| **§III Performance of public policy** | Not invoked — Owera is private-sector |
| **§VI Exercise of rights in judicial/administrative proceedings** | Litigation hold workflow (see [`incident-response-policy.md`](./incident-response-policy.md) §6) |
| **§VII Protection of life or physical safety** | Not anticipated — invoked only if a security incident materially threatens an individual's safety |
| **§VIII Protection of health** | Not invoked — Owera is not a healthcare provider |
| **§X Credit protection** | Not invoked |

Each basis chosen is documented per-treatment in the **Registro de Tratamento de Dados Pessoais (RTDP)** maintained in `compliance/rtdp/` (created on first formal review).

Consent (§I), where it is the basis, is captured per-purpose at the signup flow, is revocable at any time, and revocation is processed within 5 business days. Pre-ticked consent boxes are forbidden ([`security-policy.md`](./security-policy.md) §6).

## 3. DPO (Encarregado) designation (Art. 41)

LGPD Art. 41 mandates the controller designate an Encarregado / DPO. The DPO is the primary contact between Owera, data subjects, and ANPD.

| Field | Value |
|-------|-------|
| Name | **Rodrigo Recio** *(interim — to be replaced by a dedicated DPO on Owera's first material hiring round; Rodrigo is the founder-acting backstop)* |
| Public email | `dpo@owera.com` *(alias forwards to the active DPO; `rodrigo@recio.com` is the current authoritative endpoint while alias is pending)* |
| Postal address | Owera Software Ltda, Macapá, Amapá, Brazil *(full address in 1Password vault Owera/Legal)* |
| Hours | Acknowledgement within 5 business days; substantive response per Art. 19 §1 within 15 days |
| Public disclosure | Listed in the public privacy notice at `https://owera.com/privacy` and at `app.owera.ai` footer; ANPD registration filed within 60 days of go-live |

DPO duties (Art. 41 §2):
1. Accept complaints and communications from data subjects, providing explanations and adopting measures.
2. Receive communications from ANPD and adopt measures.
3. Guide employees and contractors on LGPD compliance.
4. Execute other functions determined by the controller or set forth in complementary norms.

The DPO has direct reporting access to the CEO and a standing 30-minute monthly slot with Owera's external counsel.

## 4. Data subject rights (Art. 18) — operational SLA

Every Art. 18 right is exposed to data subjects through a documented channel. Operational runbooks live in [`../runbooks/`](../runbooks/).

| Art. 18 right | Operational route | SLA (Owera commitment) |
|---------------|-------------------|------------------------|
| **§I Confirmation** of existence of treatment | `privacy@owera.com` reply or in-app account page | 5 business days |
| **§II Access** to the data | Self-service export at `app.owera.ai/account/export` + manual fallback via [`customer-data-export.md`](../runbooks/customer-data-export.md) | 15 days (Art. 19 §1) |
| **§III Correction** of incomplete, inaccurate, or outdated data | Self-service in the dashboard; manual via support | 5 business days |
| **§IV Anonymization, blocking, or deletion** of unnecessary/excessive/non-compliant data | Manual request via DPO; runbook [`customer-data-deletion.md`](../runbooks/customer-data-deletion.md) | 15 working days |
| **§V Portability** to another service or product | Self-service export delivers JSON + PDF in machine-readable format (Art. 19 §1) | 15 days |
| **§VI Deletion** of personal data treated based on consent | `DELETE /v1/tenants/me/data` (cloud API) or in-app account deletion → enqueues erasure job; runbook [`customer-data-deletion.md`](../runbooks/customer-data-deletion.md) | **15 working days** (LGPD strictest interpretation; satisfies GDPR Art. 12's 30 calendar days too) |
| **§VII Information about public and private entities** with which Owera shared data | Annual transparency report + ad-hoc list on request | 15 days |
| **§VIII Information about the possibility of refusing consent** and consequences | Disclosed at consent collection time; restatable on request | 5 business days |
| **§IX Revocation of consent** (Art. 8 §5) | Self-service in the dashboard's consent preferences; effective immediately, proof retained 90 days | Immediate |

**Why 15 working days for erasure when LGPD doesn't name a number:** Art. 19 §1 caps the controller's response to a portability/access request at 15 days. Art. 18 §VI (deletion) inherits this as the operational ceiling. GDPR Art. 12 is more lenient (30 calendar days, extensible by 60), so Owera adopts the stricter LGPD bar uniformly for both regimes — a single SLA simplifies the runbook.

Carve-outs (Art. 16 — retention permitted despite an erasure request):
- Compliance with legal/regulatory obligation (Receita Federal: 5y fiscal records).
- Research by research organization (Art. 16 §II) — n/a for Owera.
- Anonymized transfer to third party (Art. 16 §IV) — n/a routinely.
- Exclusive use by the controller, access denied to third parties (Art. 16 §III) — applies to the audit-log WORM store; the chain integrity is preserved, but PII fields are tokenized so the residual record is anonymized per Art. 12.

## 5. Audit-log tokenization (Art. 16 §III + Art. 12)

The audit log is WORM (write-once, read-many) with a 7-year retention under LGPD Art. 16 §I + SOC 2 evidentiary basis. When an erasure request lands, the audit-log entries referencing the tenant are not deleted; the PII fields (email, name, billing address, IP) are tokenized via HMAC-SHA-256 with a key that is destroyed in the same operation, rendering the tokens irreversible. The hash chain in `api/internal/audit/` survives intact and `audit.Log.Verify()` continues to pass.

Tokenization is itself an audited event — the operator who runs it and the SHA-256 of the destroyed key are recorded in a non-tokenizable system-level audit row.

## 6. Breach notification (Art. 48)

LGPD Art. 48 requires the controller to notify ANPD and affected data subjects of any "security incident that may cause relevant risk or damage" within a "reasonable time period" — interpreted by current ANPD guidance as **within 72 hours where feasible**, mirroring GDPR Art. 33.

Operational procedure: [`../runbooks/incident-response.md`](../runbooks/incident-response.md) §5 Communications and §6 Evidence preservation. The CISO is the accountable approver for ANPD notifications; the DPO co-signs and files via the ANPD electronic portal at `https://www.gov.br/anpd/`.

Information to include (per Art. 48 §1):
1. Description of the nature of the affected personal data.
2. Information about the data subjects involved.
3. Indication of the technical and security measures used to protect the data, observing trade and industrial secrets.
4. Risks related to the incident.
5. Reasons for delay, where notification was not immediate.
6. Measures taken to revert or mitigate the effects of the damage.

Affected data subjects are notified via email within 5 business days of confirming the incident, using the templates in `compliance/templates/breach-notification-pt-BR.md` and `breach-notification-en.md` (created on first use). Notification is in **Brazilian Portuguese** for Brazilian data subjects; English for non-Brazilian. Communications go via SES from `privacy@owera.com`.

## 7. International transfer (Arts. 33–36)

For tenants whose data subjects are outside Brazil, or where Owera's hosted infrastructure is outside Brazil (Fly machines in `gru` only; Cloudflare workers globally; Vercel edge in `gru1` + `iad1`), international transfer is grounded in:

| Art. 33 basis | Owera mechanism |
|---------------|-----------------|
| **§I Country with adequate level of protection** | Fly.io (US — assessment pending); Cloudflare (US — DPA Standard Contractual Clauses signed); Vercel (US — DPA SCCs signed); Stripe (US/IE — SCCs); AWS (US — SCCs + Article 28 DPA) |
| **§II Standard Contractual Clauses** | Default for all US-hosted operators where adequacy is not declared |
| **§III Global corporate rules** | Not applicable to Owera (single-entity) |
| **§IV Specific consent of the data subject** | Used as a fallback for tenants without SCC coverage; presented at signup |

A complete vendor map with treatment categories, transfer basis, and DPA versions is maintained in [`vendor-management-policy.md`](./vendor-management-policy.md) and exported quarterly to `compliance/audit-controls/vendor-transfer-map.yaml` (created on first export).

**EU customers** (when present) additionally trigger GDPR obligations. Owera's posture mirrors the LGPD requirements above, with the GDPR-specific additions tracked in `compliance/policies/gdpr-addendum.md` (created on first EU customer onboarding).

## 8. Data Protection Impact Assessment (DPIA / RIPD, Art. 38)

A *Relatório de Impacto à Proteção de Dados Pessoais* (RIPD) is performed before launching any treatment that may pose high risk to data subjects' rights. The triggers are:

- New SKU that processes a new category of personal data.
- Material change to legal basis for an existing treatment.
- Onboarding a new processor (vendor) that receives personal data.
- ANPD specific request.

RIPD template: `compliance/templates/ripd.md` (created on first RIPD). Each RIPD is stored in `compliance/ripds/<YYYY-MM-DD>-<slug>.md` and reviewed by the DPO + CISO + external counsel.

## 9. Personnel training (Art. 50 §2 §III)

All Owera personnel complete LGPD-awareness training at hire and annually thereafter. Content covers:
- Owera's treatment activities and legal bases.
- Data subject rights and the operational routes to honor them.
- Incident-recognition (what makes something a §48 reportable event).
- DPO contact channels for personnel questions.

Engineers with production access additionally complete:
- The audit-log WORM contract.
- The erasure runbook (so they can execute the manual path under pressure).
- Cross-border transfer mechanics.

Training records are maintained in the HR system and surfaced on auditor request.

## 10. Self-monitoring (Art. 50 §2 §I)

The controller monitors LGPD compliance via:

| Cadence | Activity | Owner |
|---------|----------|-------|
| Continuous | Audit-log WORM integrity (`audit.Log.Verify()` run nightly via the heartbeat watchdog) | SRE Lead |
| Monthly | DPO inbox review (no rights requests dropped >5 business days) | DPO |
| Quarterly | RTDP review — treatments added/removed/changed legal basis | DPO + CISO |
| Quarterly | Vendor transfer map review — DPA / SCC currency, new processors | CISO |
| Annually | Full LGPD program review with external counsel; output is a written attestation | CISO + CEO |
| On incident | Incident-implicated treatments re-validated against this policy | CISO |

## 11. Records of Treatment (Art. 37 — RTDP)

Owera maintains a Registro de Tratamento de Dados Pessoais in `compliance/rtdp/`. The RTDP includes for each treatment:

- Treatment name and purpose.
- Legal basis (mapped to Art. 7).
- Categories of personal data treated.
- Categories of data subjects.
- Categories of processors who receive the data.
- Retention period (mapped to [`data-retention-policy.md`](./data-retention-policy.md)).
- Cross-border transfers and their Art. 33 basis.
- Technical and organizational security measures.

The RTDP is the primary artifact ANPD will request on inspection.

## 12. Cross-references

- [`security-policy.md`](./security-policy.md) §2 (classification), §3 (encryption), §6 (consent)
- [`access-control-policy.md`](./access-control-policy.md) §8 (audit logging)
- [`data-retention-policy.md`](./data-retention-policy.md) (full retention schedule)
- [`vendor-management-policy.md`](./vendor-management-policy.md) (processor inventory + DPA tracking)
- [`incident-response-policy.md`](./incident-response-policy.md) §5 (ANPD/DPA notification)
- [`../runbooks/customer-data-deletion.md`](../runbooks/customer-data-deletion.md) (Art. 18 §IV/§VI execution)
- [`../runbooks/customer-data-export.md`](../runbooks/customer-data-export.md) (Art. 18 §II/§V execution)
- [`../runbooks/incident-response.md`](../runbooks/incident-response.md) §5 (Art. 48 notification)
- `api/internal/erasure/` (DELETE /v1/tenants/me/data implementation)
- `api/internal/audit/` (WORM hash-chained audit log)

## Ownership

| Role | Responsibility |
|------|----------------|
| **DPO (Encarregado)** | Day-to-day operational ownership; data subject contact; ANPD interface; annual training |
| **CISO** | Accountable for the policy as a whole; technical/organizational measures; breach response |
| **CEO** | Designates the DPO; signs the policy; ultimate accountability under Brazilian law |
| **External counsel** | Annual review; ANPD notification language sign-off |
| **All personnel** | Adherence to the consent, sensitive-data, and reporting commitments |

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security (interim DPO: Rodrigo Recio) | Initial version — names Owera Software Ltda as controller, designates interim DPO, maps Art. 7 legal bases, codifies 15-working-day erasure SLA, codifies 72h ANPD notification, lists international-transfer mechanisms |
