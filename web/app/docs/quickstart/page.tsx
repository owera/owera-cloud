import Link from "next/link";
import { RunInCompose } from "@/components/docs/run-in-compose";
import { CodeBlock } from "@/components/docs/code-block";
import { Callout } from "@/components/docs/callout";
import { CostExample } from "@/components/docs/cost-example";

export const metadata = {
  title: "Quickstart — 60 seconds to first job · Owera Docs",
};

export default function Quickstart() {
  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        GET STARTED
      </div>
      <h1>Quickstart</h1>
      <p className="lede">
        From zero to a working agent in about a minute. Two paths — one for humans,
        one for agents. Same job either way.
      </p>

      <h2>The human path</h2>
      <p>
        Open <Link href="/compose">/compose</Link>. The slider is parked at{" "}
        <b>Simple</b>. Type a prompt, click <b>Run job</b>. That&apos;s it.
      </p>
      <ol>
        <li>
          Drag the slider only when you want more — more depth, more tools, more
          budget. The cost preview updates live.
        </li>
        <li>
          After submit, you land on the job page with a live timeline. The URL is
          shareable.
        </li>
        <li>
          Want to re-run with one stop more complexity? The post-run panel offers
          that in one click — that&apos;s usually the right second move.
        </li>
      </ol>

      <Callout kind="tip">
        Keyboard works everywhere: <code>1</code>–<code>5</code> jumps to a stop,{" "}
        <code>←</code>/<code>→</code> nudges, <code>Home</code>/<code>End</code>{" "}
        snaps to ends.
      </Callout>

      <p>Pre-filled examples you can try right now:</p>
      <div className="not-prose flex flex-wrap gap-2 my-3">
        <RunInCompose
          level="simple"
          prompt="Summarize the top 5 hacker news stories with one-line takes."
        />
        <RunInCompose
          level="standard"
          prompt="Find me three recent papers on prompt caching and quote the key findings."
        />
        <RunInCompose
          level="advanced"
          prompt="Build a competitive teardown of three vector DBs (pgvector, Pinecone, Qdrant) covering pricing, throughput, and operational story."
        />
      </div>

      <h2>The agent path</h2>
      <p>
        The slider is just a UI on top of a JSON shape. An agent (Hermes, Claude,
        anything that can POST) drives the same code path.
      </p>
      <CodeBlock
        lang="bash"
        filename="curl"
        code={`curl -X POST https://app.owera.ai/api/compose \\
  -H 'authorization: Bearer $OWERA_API_KEY' \\
  -H 'content-type: application/json' \\
  -d '{
    "level": "simple",
    "sku": "triage-watch",
    "prompt": "Summarize the top 5 hacker news stories."
  }'`}
      />
      <p>You get back a job id and a share URL:</p>
      <CodeBlock
        lang="json"
        filename="response"
        code={`{
  "job_id": "job_01HF...",
  "status": "submitted",
  "shareUrl": "/jobs/job_01HF...?from=compose&level=simple",
  "rerunUrl": "/compose?level=simple&prompt=Summarize+the+top+5+hacker+news+stories.",
  "estimate": { "centsLow": 2, "centsHigh": 7, "p50ms": 4000, "p95ms": 12000, "tier": "free" }
}`}
      />

      <Callout kind="trust">
        The exact same <code>estimate</code> appears live in the slider preview as
        you drag. If we can&apos;t predict it, we won&apos;t pretend to.
      </Callout>

      <h2>What it costs (right now)</h2>
      <p>Real numbers for an empty prompt at each stop:</p>
      <ul>
        <li>
          <CostExample stop="simple" />
        </li>
        <li>
          <CostExample stop="standard" />
        </li>
        <li>
          <CostExample stop="advanced" />
        </li>
        <li>
          <CostExample stop="expert" />
        </li>
      </ul>

      <h2>Next</h2>
      <ul>
        <li>
          <Link href="/docs/concepts/complexity-slider">
            Concept — The Complexity Slider explained
          </Link>
        </li>
        <li>
          <Link href="/docs/concepts/pick-your-stop">
            Concept — Pick your stop
          </Link>
        </li>
        <li>
          <Link href="/docs/reference/api">Reference — POST /api/compose</Link>
        </li>
      </ul>
    </>
  );
}
