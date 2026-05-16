# Customer data deletion runbook

Operational procedure for LGPD Art. 18 §VI (right to deletion) and GDPR Art. 17 (right to erasure).

The policy framing is in [`../policies/data-retention-policy.md`](../policies/data-retention-policy.md) §2.

## 0. SLA

- **Acknowledge** the request within **5 business days**.
- **Complete** the deletion within **30 days** of acknowledgement.
- For court-ordered urgent erasure: **7 days**, with restic re-keying.

## 1. Receive and verify the request

```
[ ] Request arrives via privacy@owera.ai, in-app account-deletion flow, or formal channel.
[ ] Customer success ticket opened; DPO (or CISO) assigned.
[ ] Verify identity — same methods as ../runbooks/customer-data-export.md §1.
[ ] Offer a final data export ("Before we delete, would you like a copy?"). If yes,
    run customer-data-export.md first and wait for the requester to confirm receipt.
[ ] Confirm the requester understands the scope:
    - Account access ends immediately on initiation.
    - Customer payloads are deleted within 30 days.
    - Billing records are retained 5 years per Brazilian fiscal law (Receita Federal).
    - Audit-log references to the tenant remain but PII is tokenized.
[ ] Log the request in audit-log WORM store.
```

## 2. Suspend the account

```
[ ] Disable the account in Clerk/WorkOS → no further login.
[ ] Revoke active sessions and API tokens.
[ ] Pause any running jobs gracefully (let in-flight jobs complete within 1 hour, then hard-stop).
[ ] Cancel active Stripe subscriptions; do NOT delete the Stripe customer (fiscal retention).
```

## 3. Delete customer payloads

```
[ ] Run: fleetctl tenant purge --tenant-id <T> --confirm
    (TODO: implement in owera-fleet)
[ ] Verify deletion on the operator plane:
    - Job payloads under ~/.hermes/jobs/<tenant-id>/ — removed
    - Caches under ~/.hermes/cache/<tenant-id>/ — removed
    - Tenant-specific knowledge stores (vector DB, etc.) — removed
[ ] Hashes / sizes are captured before and after for the deletion record.
```

## 4. Tokenize audit-log references

The audit-log WORM store cannot be deleted (hash chain integrity + 7-year retention for SOC 2 / LGPD legal-obligation basis). What we can do:

```
[ ] Run: fleetctl audit tokenize --tenant-id <T>
    Replaces every PII field (email, name, billing address) referencing the tenant
    with an irreversible HMAC token. The token preserves chain integrity but no
    longer reveals the data.
[ ] The mapping table from token to original PII is destroyed in the same operation.
[ ] Log the tokenization event itself (this event is also subject to tokenization
    on the next request — recursion is bounded because there's nothing left to
    tokenize after pass 1).
```

LGPD Art. 16 explicitly preserves processing required for legal obligation, which covers fiscal records and the SOC 2 evidentiary need for audit logs.

## 5. Hold billing records

```
[ ] Mark the Stripe customer as inactive but do NOT delete.
[ ] Receita Federal requires 5 years of fiscal records; customer is informed of
    this in the acknowledgement email.
[ ] After 5 years, the standard purge job sweeps fiscal records out (TODO:
    implement a yearly fiscal-aging cron).
```

## 6. Backup eviction

```
[ ] Standard procedure (30-day SLA): no special action. The last restic snapshot
    containing the tenant's data falls off the daily retention window at day 30,
    matching the SLA exactly.
[ ] Urgent procedure (7-day SLA, court-ordered): re-key restic.
    1. Generate a new restic password; store in 1Password.
    2. restic key add --new-password-file /tmp/new.pw
    3. restic key remove <old-key>
    4. The 30 daily / 12 monthly snapshots cannot be decrypted; data is effectively
       evicted within 7 days of the next snapshot.
    5. Verify with restic snapshots --password-file /tmp/old.pw (must fail).
[ ] Either way, log the eviction in the WORM audit store.
```

## 7. Notify and close

```
[ ] Email the requester:
    - Confirmation of deletion completion.
    - Summary of what was deleted vs. what is retained under legal basis.
    - Reference number for the audit record.
[ ] Close the customer success ticket.
[ ] Update the WORM audit store with the completion record:
    { ts, tenant_id, request_id, scope_deleted, scope_retained, retention_basis,
      operator, hashes_before_after, completion_status: "complete" }
```

## 8. Edge cases

| Case | Procedure |
|------|-----------|
| Tenant has unpaid invoices | Notify CFO; LGPD does not allow withholding deletion for unpaid bills, but Receita Federal requires the invoices be retained 5 years regardless. Delete the customer payloads on schedule; preserve the invoices. |
| Tenant has an active legal hold | Pause the deletion; document the hold; resume when the hold lifts. Notify the requester of the delay with the legal basis. |
| Tenant is a Brazilian government entity | Specific provisions in LGPD Art. 23 may apply; route to external counsel. |
| Tenant is a minor whose parent requests deletion | Honor immediately, do not require additional verification beyond confirming the parental relationship. |
| Multiple deletion requests in succession | First request triggers the procedure; subsequent are confirmed as "deletion already in progress / complete". |

## 9. Verification

After every deletion, an SRE other than the executor verifies:

```
[ ] No tenant-id matches in: operator-plane filesystem, api SQLite cache, Stripe metadata (other than retained invoices), Clerk/WorkOS user table.
[ ] Audit-log entries referencing the tenant have all PII tokenized.
[ ] Hash chain validation passes after tokenization.
[ ] Confirmation email was sent and received.
```

Verification result is stored in the audit record.

## Cross-references

- [`../policies/data-retention-policy.md`](../policies/data-retention-policy.md) — retention schedule + tokenization basis
- [`customer-data-export.md`](./customer-data-export.md) — companion runbook for export requests
- [`../policies/access-control-policy.md`](../policies/access-control-policy.md) — break-glass for verification access
- [`incident-response.md`](./incident-response.md) — if a deletion request reveals a prior breach

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
