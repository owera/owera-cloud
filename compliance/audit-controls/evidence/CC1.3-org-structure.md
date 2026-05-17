# CC1.3 — Organization structures, reporting lines, authorities

**Control description:** Organization structures, reporting lines, authorities.

**Evidence source:** compliance/policies/access-control-policy.md §3

**Owner:** CISO

**Status:** see soc2-cc.yaml `status` field for CC1.3.

## What the auditor pulls

The auditor reads the file/path/log-query named in **Evidence source** and verifies:

- The artifact exists at the named location at the audit-window point-in-time.
- Its contents match the description above.
- Any cross-references it makes (other policies / runbooks / config files) resolve.

## TODO before SOC 2 Type 1 window opens

- [ ] **CISO** to confirm the evidence source is current as of the audit window start.
- [ ] If the source is out-of-tree (1Password / HR SaaS / Stripe console), capture a point-in-time screenshot and check it into `compliance/audit-controls/evidence/CC1.3-screenshots/` 7 days before the auditor's read date.
- [ ] If the source is a log query, persist the query string and expected row shape here so the auditor can re-run it.
- [ ] Link any related post-incident review that materially exercised this control.

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security (T18.4 mapping pass) | Placeholder created during evidence-path mapping; substantive evidence content pending. |
