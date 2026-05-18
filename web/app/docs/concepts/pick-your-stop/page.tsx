import Link from "next/link";
import { RunInCompose } from "@/components/docs/run-in-compose";
import { Callout } from "@/components/docs/callout";

export const metadata = {
  title: "Pick your stop · Owera Docs",
};

export default function PickYourStop() {
  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        CONCEPT
      </div>
      <h1>Pick your stop</h1>
      <p className="lede">
        A practical guide to choosing which slider stop fits the job. If in doubt
        — start one stop lower than feels right. The post-run panel will offer to
        re-run a stop higher, and that&apos;s usually the cheapest learning path.
      </p>

      <h2>Simple — when you want an answer, not an opinion</h2>
      <p>
        One prompt, one shot. Best for: summarization, classification, a quick
        rewrite, a one-line lookup. No tools, no chain. Most consumer-shaped
        prompts (&quot;summarize this&quot;, &quot;is this sentence
        grammatical&quot;, &quot;rewrite in plain English&quot;) belong here.
      </p>
      <RunInCompose
        level="simple"
        prompt="Rewrite the following in plain English: 'Per our previous correspondence...'"
        label="Try Simple"
      />

      <h2>Standard — when you want a smart answer with citations</h2>
      <p>
        A handful of tools and a slightly bigger budget. Best for: research
        questions where a one-shot answer would hallucinate, light data
        extraction, or any task that benefits from a single web search.
      </p>
      <RunInCompose
        level="standard"
        prompt="What's the current state of GPU pricing across the major cloud providers? Cite sources."
        label="Try Standard"
      />

      <h2>Advanced — when you want a research memo or a multi-step plan</h2>
      <p>
        Multi-step agent loop with retrieval. Budget cap and latency cap visible.
        Best for: competitive teardowns, technical comparisons, multi-source
        synthesis, anything that benefits from a structured plan-then-execute.
      </p>
      <RunInCompose
        level="advanced"
        prompt="Compare pgvector, Pinecone, and Qdrant: pricing, throughput, operational story. Build a 300-word memo with a recommendation."
        label="Try Advanced"
      />

      <h2>Expert — when you want orchestration and an eval pass</h2>
      <p>
        Multiple agents working in concert with an eval gate. Best for: code
        generation with a test pass, marketing fan-outs across personas, ETL
        with validation, anything where you want the system to second-guess
        itself before handing you the result.
      </p>
      <RunInCompose
        level="expert"
        prompt="Write a TypeScript function that deduplicates an array of objects by a key, then write 5 unit tests for it. Run the tests."
        label="Try Expert"
      />

      <h2>Custom — when you know exactly what you want</h2>
      <p>
        Full surface, no rails. Best for: integration builders, API-first users,
        and anyone porting an existing prompt template into Owera. You get
        everything the API exposes, and we trust you to know what to do with it.
      </p>

      <Callout kind="tip">
        Heuristic: if you can describe the task in one sentence and you know what
        a good answer looks like, start at <b>Simple</b>. If you&apos;d
        normally hand it to a junior employee with two hours, start at{" "}
        <b>Advanced</b>. If you&apos;d normally hand it to a small team for a
        morning, start at <b>Expert</b>.
      </Callout>

      <h2>Cost-first vs quality-first</h2>
      <p>
        Two reasonable defaults depending on what you care about:
      </p>
      <ul>
        <li>
          <b>Cost-first:</b> always start at <b>Simple</b>. Use the re-run-one-stop-higher
          button only when the result misses. Cheapest path to the answer.
        </li>
        <li>
          <b>Quality-first:</b> start at <b>Advanced</b>. Drop to{" "}
          <b>Standard</b> if you notice the agent doing more than you asked.
          Highest-quality path for important work.
        </li>
      </ul>

      <h2>Read next</h2>
      <ul>
        <li>
          <Link href="/docs/concepts/cost-and-pricing">
            Cost & pricing model
          </Link>
        </li>
        <li>
          <Link href="/docs/reference/api">
            API reference — POST /api/compose
          </Link>
        </li>
      </ul>
    </>
  );
}
