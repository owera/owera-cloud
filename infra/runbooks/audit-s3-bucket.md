# Audit WORM S3 bucket — provision + cutover

Stand up the S3 bucket that receives WORM audit-log entries from the apiserver, then flip the Fly secrets that activate `S3WORMStreamer`. Idempotent; safe to re-run partial sections after a failure.

**Owner:** infra operator (you, today).
**Prerequisite:** AWS account with permission to create buckets + IAM users. Stripe live-mode cutover is independent and can happen before or after.
**Estimated time:** 15 minutes once AWS creds are in hand.

---

## 1. Decide the names

| Value | Recommended |
|---|---|
| Bucket name | `owera-audit-prod` (must be globally unique; suffix with `-<acct>` if taken) |
| Region | `us-east-1` (matches Stripe + Fly latency profile) |
| Retention | 2555 days = 7 years (SOC 2 / HIPAA baseline; the apiserver rejects values < 365) |
| IAM user | `owera-cloud-audit-writer` |
| Object key prefix | `audit/` (hard-coded in `S3WORMStreamer.wormKey`) |

Capture decisions in 1Password ("Owera audit S3 bucket").

---

## 2. Create the bucket with Object Lock enabled

**Object Lock can only be enabled at bucket creation**; existing buckets cannot opt in. If a prior attempt created a bucket *without* Object Lock, delete it and recreate.

```bash
export OWERA_AUDIT_BUCKET=owera-audit-prod
export OWERA_AUDIT_REGION=us-east-1

aws s3api create-bucket \
  --bucket "$OWERA_AUDIT_BUCKET" \
  --region "$OWERA_AUDIT_REGION" \
  --object-lock-enabled-for-bucket
```

(In `us-east-1`, no `--create-bucket-configuration` is needed; all other regions require `--create-bucket-configuration LocationConstraint=<region>`.)

Set the default Object Lock configuration so per-object PUTs without retention headers still default to GOVERNANCE:

```bash
aws s3api put-object-lock-configuration \
  --bucket "$OWERA_AUDIT_BUCKET" \
  --object-lock-configuration '{
    "ObjectLockEnabled": "Enabled",
    "Rule": { "DefaultRetention": { "Mode": "GOVERNANCE", "Days": 2555 } }
  }'
```

Belt-and-suspenders: block all public access.

```bash
aws s3api put-public-access-block \
  --bucket "$OWERA_AUDIT_BUCKET" \
  --public-access-block-configuration \
    BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
```

Enable versioning (required by Object Lock — `put-object-lock-configuration` auto-enables it, but verify):

```bash
aws s3api get-bucket-versioning --bucket "$OWERA_AUDIT_BUCKET"
# expect: { "Status": "Enabled" }
```

---

## 3. Create the IAM writer user + policy

Least-privilege policy: PutObject + PutObjectRetention on the audit prefix only. **No DeleteObject, no BypassGovernanceRetention** — those are the explicit non-grants that make the writer compliance-safe.

Save as `audit-writer-policy.json`:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AuditPutOnly",
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:PutObjectRetention",
        "s3:PutObjectLegalHold"
      ],
      "Resource": "arn:aws:s3:::owera-audit-prod/audit/*"
    },
    {
      "Sid": "BucketProbe",
      "Effect": "Allow",
      "Action": [ "s3:GetBucketLocation" ],
      "Resource": "arn:aws:s3:::owera-audit-prod"
    }
  ]
}
```

Apply:

```bash
aws iam create-user --user-name owera-cloud-audit-writer
aws iam put-user-policy \
  --user-name owera-cloud-audit-writer \
  --policy-name owera-audit-put-only \
  --policy-document file://audit-writer-policy.json
aws iam create-access-key --user-name owera-cloud-audit-writer
# capture AccessKeyId + SecretAccessKey → 1Password ("Owera audit S3 bucket")
```

Optional read-only reviewer role for compliance audits (separate user, separate creds): `s3:GetObject` + `s3:ListBucket` on the same prefix. Not needed for the apiserver wire-up.

---

## 4. Local smoke against MinIO (optional, ≈ 5 min)

Before pointing the apiserver at real AWS, verify the streamer + bucket layout end-to-end against MinIO with Object Lock:

```bash
docker run -d -p 9000:9000 --name owera-audit-minio \
  -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data

# create MinIO client + Object-Locked bucket
mc alias set local http://localhost:9000 minioadmin minioadmin
mc mb --with-lock local/owera-audit-dev

# boot apiserver against MinIO
cd /Users/claw3/owera-cloud/api
OWERA_AUDIT_S3_BUCKET=owera-audit-dev \
OWERA_AUDIT_S3_ENDPOINT=http://localhost:9000 \
OWERA_AUDIT_S3_REGION=us-east-1 \
OWERA_AUDIT_S3_RETENTION_DAYS=365 \
AWS_ACCESS_KEY_ID=minioadmin \
AWS_SECRET_ACCESS_KEY=minioadmin \
go run ./cmd/apiserver --db /tmp/audit-smoke.db &

# trigger an audit event (one job submission via the chi router) then:
mc ls --recursive local/owera-audit-dev/audit/
# expect: audit/<tenant>/<YYYY-MM-DD>/<hash>.json

# teardown
kill %1
docker rm -f owera-audit-minio
```

If MinIO returns objects with the correct path layout and retention metadata (`mc stat`), the wire-up is good.

---

## 5. Fly cutover

Apply secrets one shot — Fly redeploys after `secrets set`:

```bash
fly secrets set --app owera-agentic-api \
  OWERA_AUDIT_S3_BUCKET=owera-audit-prod \
  OWERA_AUDIT_S3_REGION=us-east-1 \
  OWERA_AUDIT_S3_RETENTION_DAYS=2555 \
  AWS_ACCESS_KEY_ID=<from step 3> \
  AWS_SECRET_ACCESS_KEY=<from step 3>
```

(Do **not** set `OWERA_AUDIT_S3_ENDPOINT` for AWS; the default `https://s3.<region>.amazonaws.com` is right. Only set it for MinIO or other S3-compatible stores.)

Verify the boot-log fingerprint shows the new wiring:

```bash
fly logs --app owera-agentic-api | grep -E "apiserver: billing="
# expect line like:
#   apiserver: billing=stripe, ledger=tunnel(...), rpc=tunnel(...), auth=clerk(...), audit=s3 (owera-audit-prod@us-east-1, 2555d), default_cap_cents=20000
```

Submit one paid job and confirm in S3:

```bash
aws s3 ls --recursive s3://owera-audit-prod/audit/ | head
# expect: one .json object whose key matches audit/<tenant>/<today>/<hash>.json
```

---

## 6. Rollback

Unset any of the OWERA_AUDIT_S3_* secrets and Fly redeploys to SQLite-only mode (WORM triggers at the DB layer remain enforced — the S3 ship just stops):

```bash
fly secrets unset --app owera-agentic-api OWERA_AUDIT_S3_BUCKET
# boot log will revert to: ... audit=sqlite-only ...
```

The pre-rollback objects in S3 are untouched and remain under Object Lock. Rolling forward later re-attaches; the streamer is idempotent on a re-PUT only after retention expires (Object Lock blocks duplicate writes inside the window — this is the desired tamper-rejection property).

---

## 7. Out of scope (separate tasks, not blocking V1)

- **Lifecycle policy → Glacier** (cost optimisation; cheap to add later via `aws s3api put-bucket-lifecycle-configuration`).
- **Hash-chain verifier cron** that fetches the day's objects from S3 and replays them through `audit.Verify` to catch in-flight tampering — separate skill, captured in `~/owera-fleet/docs/roadmap.md`.
- **Replication to a second region** — only needed if RPO commitments demand it.
- **CloudTrail on the bucket itself** — useful for forensics but adds noise; consider when there's a second human with bucket access.

---

## Cross-references

- Wire-up code: `api/cmd/apiserver/main.go` `chooseWiring()`, `S3WORMStreamer` section.
- Publisher implementation: `api/internal/audit/worm.go:126-171` (object-lock headers).
- SigV4 transport: `api/internal/audit/sigv4_transport.go`.
- Acceptance tests: `api/internal/audit/worm_publisher_test.go`.
- Secrets baseline: `infra/secrets-manifest.md`.
