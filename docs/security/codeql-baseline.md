# CodeQL baseline

GitHub's CodeQL scanner runs on every push to `main`, every PR targeting
`main` or `wave-*`, and on a weekly cron (Monday 06:00 UTC). It analyzes both
Go (`api/`) and JavaScript/TypeScript (`web/`, `status/app/`) sources with the
`security-and-quality` query suite.

## Purpose of this file

Day-one findings get triaged here; mute or fix; don't leave the workflow red.

When CodeQL produces alerts on the first run, each one lands in one of three
buckets:

1. **Fix** — real bug or vulnerability. File a ticket, land a patch, the alert
   closes itself on the next scan.
2. **Dismiss in-GitHub** — false positive, test-only code, or an intentional
   pattern. Dismiss with a reason via the Security tab; record the dismissal
   below for posterity.
3. **Baseline here** — finding is real but pre-existing, low-severity, and not
   blocking the public-repo flip. Capture it in the table below so future
   contributors know it's been seen and accepted.

The goal is a green CodeQL badge on `main` within one week of the first run.
Anything still red after that gets escalated.

## Known findings (acceptable, with rationale)

| Alert ID | Rule | Language | Path | First seen | Rationale | Owner |
| --- | --- | --- | --- | --- | --- | --- |
| _(empty — populate after first scan)_ | | | | | | |

## How to add a row

1. Open the alert in GitHub's Security → Code scanning tab.
2. Copy the alert number (e.g. `#42`) into the `Alert ID` column.
3. Copy the rule short name (e.g. `go/sql-injection`) into `Rule`.
4. Note the file path and the reason it's acceptable (e.g. "test fixture",
   "intentional dev-only assertion", "third-party generated code").
5. Tag an owner so we know who reviews it if the rule changes.

If the table grows past ~10 rows, that's a signal we're accepting too much
risk; revisit and fix the underlying issues.
