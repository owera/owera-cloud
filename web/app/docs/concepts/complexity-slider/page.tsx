import Link from "next/link";
import { RunInCompose } from "@/components/docs/run-in-compose";
import { Callout } from "@/components/docs/callout";
import { CostExample } from "@/components/docs/cost-example";

export const metadata = {
  title: "The Complexity Slider · Owera Docs",
};

export default function ComplexitySliderConcept() {
  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        CONCEPT
      </div>
      <h1>The Complexity Slider</h1>
      <p className="lede">
        One control governs the entire surface: a discrete slider with five named
        stops. Each stop dials four things at once — what the UI shows, how
        deeply the agent thinks, what it costs, and which pricing tier you&apos;re
        on.
      </p>

      <h2>Why a slider?</h2>
      <p>
        Most AI products give you twelve knobs and call it &quot;configurable.&quot;
        We give you one. Internally we still set the twelve knobs — but tied
        together along the axis that actually matters: <b>how much work do you
        want done?</b>
      </p>

      <h2>The five stops</h2>
      <table>
        <thead>
          <tr>
            <th>Stop</th>
            <th>You see</th>
            <th>It runs</th>
            <th>Typical cost (empty prompt)</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>
              <b>Simple</b>
            </td>
            <td>A prompt and a button.</td>
            <td>Single-shot call. Default model.</td>
            <td>
              <CostExample stop="simple" />
            </td>
          </tr>
          <tr>
            <td>
              <b>Standard</b>
            </td>
            <td>+ tool toggles, output shape.</td>
            <td>Hermes + 1–2 tools, ~30s budget.</td>
            <td>
              <CostExample stop="standard" />
            </td>
          </tr>
          <tr>
            <td>
              <b>Advanced</b>
            </td>
            <td>+ model picker, depth, budget cap.</td>
            <td>Multi-step agent loop with retrieval.</td>
            <td>
              <CostExample stop="advanced" />
            </td>
          </tr>
          <tr>
            <td>
              <b>Expert</b>
            </td>
            <td>+ agent chain, branching, full tools.</td>
            <td>Multi-agent orchestration with eval pass.</td>
            <td>
              <CostExample stop="expert" />
            </td>
          </tr>
          <tr>
            <td>
              <b>Custom</b>
            </td>
            <td>Full JSON. No safety rails.</td>
            <td>Anything the API exposes.</td>
            <td>
              <CostExample stop="custom" />
            </td>
          </tr>
        </tbody>
      </table>

      <Callout kind="cost">
        Cost previews are <b>ranges</b>, not point estimates. The slider preview
        and the post-run bill are computed from the same SKU pricing. If the
        billed cents land outside the previewed range, that&apos;s a bug — please
        tell us.
      </Callout>

      <h2>Every state is a URL</h2>
      <p>
        Drag the slider and you&apos;ll see the URL update:{" "}
        <code>?level=advanced&amp;prompt=…&amp;tools=web,code</code>. Copy that
        URL anywhere — your notes, a colleague&apos;s inbox, a script — and the
        slider lands at exactly the same configuration. There&apos;s no hidden
        state. The URL is the state.
      </p>
      <p>The same shape is the body of the public API.</p>

      <Callout kind="trust">
        We do this on purpose. A surface where humans and agents see the same
        thing means: no &quot;dev mode&quot; vs &quot;UI mode&quot; drift, no
        forked code paths, and your agent can show you exactly what it would do
        before doing it.
      </Callout>

      <h2>Tier gates</h2>
      <p>
        Stops <b>Advanced</b> and beyond require a signed-in account. Stops{" "}
        <b>Expert</b> and beyond require a paid plan. The gate fires inline — no
        modal interrupt — and the slider thumb visibly resists past the gate.
      </p>
      <p>
        URL-based bypass doesn&apos;t work: the gate is enforced server-side both
        when the page renders and when the API receives the POST.
      </p>

      <h2>Try it</h2>
      <div className="not-prose flex flex-wrap gap-2 my-3">
        <RunInCompose
          level="simple"
          prompt="Three bullet-point takeaways from today's HN front page."
        />
        <RunInCompose
          level="advanced"
          prompt="Build a research memo on the state of small language models, citing five recent papers."
          label="Try Advanced"
        />
      </div>

      <h2>Read next</h2>
      <ul>
        <li>
          <Link href="/docs/concepts/pick-your-stop">
            Pick your stop — when to use which level
          </Link>
        </li>
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
