import Link from "next/link";
import { Callout } from "@/components/docs/callout";
import { RunInCompose } from "@/components/docs/run-in-compose";

export const metadata = {
  title: "Anatomy of a job · Owera Docs",
};

export default function AnatomyOfAJob() {
  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        CONCEPT
      </div>
      <h1>Anatomy of a job</h1>
      <p className="lede">
        An Owera job has five attributes. Each one is a choice you make in the
        composer. Once hired, you can re-run it, edit it, or schedule it to
        repeat — without composing from scratch.
      </p>

      <h2>1 · Outcome — what kind of work</h2>
      <p>
        Six archetypes cover the common shapes:{" "}
        <b>Research</b>, <b>Triage</b>, <b>Brief</b>, <b>Build</b>,{" "}
        <b>Watch</b>, and <b>Custom</b>. Each renders an archetype-specific
        form so you fill structured fields instead of staring at a blank
        textarea.
      </p>
      <Callout kind="tip">
        Picking an archetype seeds sensible defaults for everything else —
        SKU, Quality dial, suggested cadence. You can override any of them
        before hiring.
      </Callout>

      <h2>2 · Recipe — the agent and the Quality dial</h2>
      <p>
        The Quality dial (the slider) lives in step 2 of the composer. Five
        stops:{" "}
        <code>simple</code>, <code>standard</code>, <code>advanced</code>,{" "}
        <code>expert</code>, <code>custom</code>. Each stop is a fixed
        multiplier on cost and depth — see{" "}
        <Link href="/docs/concepts/cost-and-pricing">Cost &amp; pricing</Link>.
      </p>

      <h2>3 · Cadence — when it runs</h2>
      <p>Step 3 of the composer. Four kinds:</p>
      <ul>
        <li>
          <b>Run once</b> — ad-hoc.
        </li>
        <li>
          <b>Daily</b> at a chosen time (your local timezone).
        </li>
        <li>
          <b>Weekly</b> on chosen weekdays.
        </li>
        <li>
          <b>Cron</b> — five-field cron expression for power users.
        </li>
      </ul>
      <Callout kind="trust">
        For recurring jobs, the composer shows you a{" "}
        <b>projected monthly cost range</b> based on the per-run estimate ×
        runs per month. Real billed cents always land in the previewed range,
        or it&apos;s a bug.
      </Callout>

      <h2>4 · Delivery — where the output lands</h2>
      <p>
        Step 3 also picks delivery: <b>Dashboard</b> (default — always works),{" "}
        <b>Email</b>, <b>Slack</b>, or <b>Webhook</b>. Non-dashboard targets
        require an address / channel / URL.
      </p>

      <h2>5 · Identity — name + template</h2>
      <p>
        Step 4 reviews the composition in plain language. You can edit the
        synthesized job name, and toggle <b>Save as template</b> so the job
        appears in your Jobs list for one-click re-hire.
      </p>

      <h2>The re-hire loop</h2>
      <p>
        Every job has a <b>Run again</b>, <b>Edit &amp; re-hire</b>, and{" "}
        <b>Schedule</b> action on its detail page. Templates saved during
        composition show up under <Link href="/jobs">Your jobs</Link> for
        one-click access.
      </p>

      <h2>Try it</h2>
      <div className="not-prose flex flex-wrap gap-2 my-3">
        <RunInCompose
          archetype="brief"
          level="standard"
          prompt="Daily 300-word brief on AI safety papers + open-source releases."
          label="Open a brief"
        />
        <RunInCompose
          archetype="watch"
          level="standard"
          prompt="Watch HN + r/MachineLearning for posts that mention Owera or a competitor by name."
          label="Open a watch"
        />
      </div>

      <h2>Read next</h2>
      <ul>
        <li>
          <Link href="/docs/concepts/complexity-slider">
            The Complexity Slider (the Quality dial in detail)
          </Link>
        </li>
        <li>
          <Link href="/docs/concepts/cost-and-pricing">
            Cost &amp; pricing
          </Link>
        </li>
        <li>
          <Link href="/docs/reference/api">API reference</Link>
        </li>
      </ul>
    </>
  );
}
