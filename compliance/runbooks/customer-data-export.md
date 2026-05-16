# Customer data export runbook

Operational procedure for LGPD Art. 18 §V / §VIII (right of access, portability) and GDPR Art. 20 (data portability) requests.

The policy framing is in [`../policies/data-retention-policy.md`](../policies/data-retention-policy.md) §6.

## 0. SLA

- **Acknowledge** the request within **5 business days**.
- **Deliver** the export within **15 days** of acknowledgement (LGPD Art. 19 §1, calendar days — strict).

## 1. Receive and verify the request

```
[ ] Request arrives via privacy@owera.ai (the standing channel), inline support ticket,
    or — rarely — via post / official notification.
[ ] Customer success ticket is opened; the DPO (or CISO acting as DPO until appointed)
    is added as the assignee.
[ ] Verify the requester's identity. Methods (in order of preference):
    1. Authenticated session: requester logs into app.owera.ai and clicks "Request my data"
       — identity verified by auth provider.
    2. Reply to the email associated with the account, from that same email address.
    3. Identity document + signed statement (last resort; flag for legal review).
[ ] Log the request in audit-log WORM store: { ts, tenant_id, requester, channel, status }.
```

If the requester is a third party (e.g. an attorney), require notarized authorization before proceeding.

## 2. Scope the export

Categories the customer is entitled to export (per [`../policies/data-retention-policy.md`](../policies/data-retention-policy.md) §1):

- Account metadata
- Billing records (invoices, transactions)
- Customer payloads still within retention
- Agent job metadata (job ids, statuses, timings)
- Consent records

Categories explicitly NOT exported:

- Other customers' data (obviously)
- Owera's internal operational logs that happen to reference the customer
- Audit-log entries (covered separately under access requests; the customer can request a confirmation of WHAT is logged but not the raw chain entries — see §6 below)

## 3. Generate the export

```
[ ] Run: fleetctl export tenant --tenant-id <T> --out /tmp/export-<T>-<YYYY-MM-DD>.zip
    (TODO: implement in owera-fleet; until then, the SRE-on-duty exports each
    category via the api admin endpoints and bundles manually)
[ ] The export zip contains:
    /account.json
    /billing.json
    /payloads/<job-id>/{input.json, output.json}
    /jobs.json
    /consent.json
    /README.md          (human-readable explanation of the structure)
    /summary.pdf        (human-readable summary)
[ ] Verify the export by inspecting account.json — does it match what the customer
    sees in their app dashboard?
```

## 4. Deliver the export

```
[ ] Generate a single-use signed download link, expiring in 7 days, from an isolated
    delivery bucket (s3://owera-data-exports/<tenant-id>/<request-id>/).
[ ] Email the link to the requester's verified email. Include:
    - A note that the link is single-use and expires in 7 days.
    - SHA-256 of the zip, for integrity verification.
    - Instructions for re-requesting if the link fails.
[ ] Log delivery in the WORM audit store.
[ ] After 30 days, the export zip is destroyed (lifecycle policy on the bucket).
```

For high-trust enterprise customers, the delivery may be via 1Password share or SFTP to an agreed endpoint — the channel is captured in the customer's contract.

## 5. Edge cases

| Case | Procedure |
|------|-----------|
| Customer requests export of a deleted account | The deletion runbook ([`customer-data-deletion.md`](./customer-data-deletion.md)) requires a "final export" be offered at deletion time; after the deletion grace window (90 days) the data is gone and we can only confirm the deletion. |
| Customer requests export of data older than retention | We can only export what we have. The retention policy ([`../policies/data-retention-policy.md`](../policies/data-retention-policy.md)) is the source of truth on what's available; document the gap in the response. |
| Customer requests export for a tenant they don't control (employee leaving a company) | Refuse and redirect — the request must come from the tenant owner of record. |
| Subpoena / law enforcement request | Do NOT honor as a data-export request. Route to external counsel; they decide whether the request is valid under Brazilian law (and EU law if applicable). |

## 6. Audit log access requests

A customer may ask "what does Owera's audit log say about my account?" This is a narrower question than a full export. Procedure:

```
[ ] Generate a summary of audit-log categories that reference the tenant (counts,
    not raw rows).
[ ] If the customer wants specific rows, the DPO reviews each row for:
    - Inclusion of other-tenant or system-internal data → redact.
    - Hash-chain integrity → must remain verifiable.
[ ] Deliver the redacted set via the same single-use signed-link mechanism.
```

The audit-log WORM store itself is never exported in full to a customer — the hash chain references internal system state and other tenants' audit events.

## 7. Tracking

Every export request is tracked in:

- The audit-log WORM store (request receipt, scope, delivery, expiry).
- A quarterly metrics report (count of requests, p50/p95 SLA performance, gaps).

The CISO reviews the metrics in quarterly access reviews.

## Cross-references

- [`../policies/data-retention-policy.md`](../policies/data-retention-policy.md) — what's retained, what's exportable
- [`customer-data-deletion.md`](./customer-data-deletion.md) — companion runbook for deletion requests
- [`../policies/security-policy.md`](../policies/security-policy.md) §6 — consent framing

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
