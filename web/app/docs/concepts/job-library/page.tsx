import Link from "next/link";
import { Callout } from "@/components/docs/callout";
import { FUNCTIONS } from "@/lib/compose/functions";
import { JOB_CATALOG, killerJobs } from "@/lib/compose/catalog";

export const metadata = {
  title: "The job library · Owera Docs",
};

export default function JobLibraryConcept() {
  const total = JOB_CATALOG.length;
  const killers = killerJobs().length;
  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        CONCEPT
      </div>
      <h1>The job library</h1>
      <p className="lede">
        Owera Agentic is a library of <b>{total} hireable jobs</b> across{" "}
        {FUNCTIONS.length} business functions. You don&apos;t write a prompt —
        you hire a job. {killers} of them are jobs SMBs pay $50–$500/month
        for without flinching; those are marked with a star in the library.
      </p>

      <h2>Why &quot;jobs&quot; and not prompts</h2>
      <p>
        A prompt is a one-shot request. A <b>job</b> is a durable position in
        your workflow: it has a trigger, a source, an action, an output, and
        an approval gate. It runs on a cadence, escalates when stuck, and
        produces a value receipt you accept or reject per item. We charge
        only for items you accept.
      </p>

      <Callout kind="tip">
        Browse the library by function (Sales, Finance, Customer Success, …)
        from the rail on <Link href="/compose">/compose</Link>. Star-marked
        jobs are the most-hired by similar businesses.
      </Callout>

      <h2>The five slots</h2>
      <p>
        Every job is described by five slots — Owera&apos;s contract with the
        agent. Once you hire a job, you can edit any slot before it runs:
      </p>
      <table>
        <thead>
          <tr>
            <th>Slot</th>
            <th>What it answers</th>
            <th>Options</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>WHEN</td>
            <td>When does the agent run?</td>
            <td>Schedule / event / inbox / manual / threshold</td>
          </tr>
          <tr>
            <td>READ</td>
            <td>What does it have access to?</td>
            <td>HubSpot, Gmail, Stripe, Slack, Mixpanel, the web…</td>
          </tr>
          <tr>
            <td>DO</td>
            <td>What does it do, in plain English?</td>
            <td>Research, draft, classify, monitor, reconcile…</td>
          </tr>
          <tr>
            <td>DELIVER</td>
            <td>Where does the result land?</td>
            <td>Memo, table, Slack message, email draft, ticket, alert</td>
          </tr>
          <tr>
            <td>CONFIRM</td>
            <td>How much autonomy does it have?</td>
            <td>
              Autonomous · Draft-for-approval · Always-ask · Confidence-gated
            </td>
          </tr>
        </tbody>
      </table>

      <h2>How we charge</h2>
      <p>
        Each job has a <b>suggested subscription</b> (e.g. $299/mo for
        outbound prospecting) and a <b>billing unit</b> (e.g. per qualified
        lead). Owera charges for items you accept on the value receipt —
        not for attempted work, not per token, not per run. If the agent
        produces 50 leads and you accept 32, you&apos;re billed for 32.
      </p>

      <Callout kind="trust">
        We charge for accepted output, not attempted work. This is the
        structural reason an Owera agent feels different from a generic
        LLM call — every run produces a receipt, every receipt has a human
        accept-or-reject decision, and the receipt is the billing artifact.
      </Callout>

      <h2>The deep-agent execution pattern</h2>
      <ul>
        <li>
          <b>Named milestones</b>, not raw logs. The shipment tracker shows
          things like &quot;Filtered to 73 matching ICP rules&quot; and
          &quot;Enriched 73 with funding &amp; headcount&quot; — not
          tool-call traces.
        </li>
        <li>
          <b>Blocking vs non-blocking questions.</b> The agent asks before
          sending anything external; it makes judgment calls and flags them
          on internal work.
        </li>
        <li>
          <b>Resumability.</b> Long jobs checkpoint state. A crashed worker
          resumes where it left off, not from zero.
        </li>
        <li>
          <b>Value receipts</b> close the loop: accept/reject per item, only
          accepted items count toward billing.
        </li>
      </ul>

      <h2>For agents</h2>
      <p>
        The library is also API-discoverable. Every job card deep-links to{" "}
        <code>/compose/build?job=&lt;id&gt;</code>, and the same id is
        accepted by <code>POST /api/compose</code> in the{" "}
        <code>owera_job_id</code> input. Schema:{" "}
        <Link href="/api/compose/schema">/api/compose/schema</Link>.
      </p>

      <h2>Read next</h2>
      <ul>
        <li>
          <Link href="/docs/concepts/anatomy-of-a-job">Anatomy of a job</Link>{" "}
          — the five attributes in detail
        </li>
        <li>
          <Link href="/docs/concepts/complexity-slider">
            The Complexity Slider (the Quality dial)
          </Link>
        </li>
        <li>
          <Link href="/docs/concepts/cost-and-pricing">
            Cost &amp; pricing
          </Link>
        </li>
      </ul>
    </>
  );
}
