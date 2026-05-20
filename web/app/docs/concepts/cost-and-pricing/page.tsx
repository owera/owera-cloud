import Link from "next/link";
import { Callout } from "@/components/docs/callout";
import { CostExample } from "@/components/docs/cost-example";

export const metadata = {
  title: "Cost & pricing model · Owera Docs",
};

export default function CostAndPricing() {
  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        CONCEPT
      </div>
      <h1>Cost & pricing model</h1>
      <p className="lede">
        Every cost number you see — preview, post-run, monthly invoice — is
        computed from the same inputs: SKU base price × stop factor × any tool
        amplification. No hidden multipliers. No surprise fees.
      </p>

      <h2>How a cost preview is built</h2>
      <ol>
        <li>
          We take the <b>base price</b> for the SKU you selected (visible in the
          <Link href="/docs/reference/skus"> SKU catalog</Link>).
        </li>
        <li>
          Multiply by the <b>stop factor</b> — a fixed multiplier per slider stop
          that approximates how much work that depth implies.
        </li>
        <li>
          If you have more than two tools enabled, we add a small per-tool
          amplification (15% per extra tool past two).
        </li>
        <li>
          Take that center value and expand it into a <b>±35% range</b>. The
          preview shows that range. Real billed cents almost always lands
          inside.
        </li>
        <li>
          If you set a <b>max budget</b>, we hard-cap the high end of the range
          to your budget. The agent will stop before exceeding it.
        </li>
      </ol>

      <Callout kind="cost">
        Real numbers, right now, for an empty prompt:{" "}
        <CostExample stop="simple" />, <CostExample stop="standard" />,{" "}
        <CostExample stop="advanced" />, <CostExample stop="expert" />.
      </Callout>

      <h2>Why a range, not a number?</h2>
      <p>
        A single number on a preview is a promise we can&apos;t keep — token
        usage varies with the prompt, retries happen, models fluctuate.
        Pretending otherwise leads to first-invoice surprise, which kills trust.
        A range is honest: it tells you the high end you might owe and the low
        end you might luck into.
      </p>

      <h2>Pricing tiers</h2>
      <p>
        The slider stops map to four tiers. Higher stops require higher tiers:
      </p>
      <table>
        <thead>
          <tr>
            <th>Stops</th>
            <th>Tier</th>
            <th>Who it&apos;s for</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Simple, Standard</td>
            <td>
              <b>Free</b>
            </td>
            <td>Try Owera with no commitment. Reasonable monthly cap.</td>
          </tr>
          <tr>
            <td>Advanced</td>
            <td>
              <b>Pro</b>
            </td>
            <td>
              Individual operators and small teams. 14-day trial, no card
              required.
            </td>
          </tr>
          <tr>
            <td>Expert</td>
            <td>
              <b>Team</b>
            </td>
            <td>Teams running multi-agent orchestration at scale.</td>
          </tr>
          <tr>
            <td>Custom</td>
            <td>
              <b>Enterprise</b>
            </td>
            <td>API-first integrators, custom SLAs, audit log, SSO.</td>
          </tr>
        </tbody>
      </table>

      <h2>How we bill</h2>
      <p>
        Every job is metered in USD cents and posted to your{" "}
        <Link href="/billing">billing portal</Link>. Each run produces a ledger
        entry visible on the job timeline — you can see exactly what each step
        cost, in order.
      </p>

      <Callout kind="trust">
        The preview range and the ledger entries use the same numbers. If you
        ever see a billed total above the preview high, that&apos;s a bug — open
        a ticket from the <Link href="/support">support page</Link> and we&apos;ll
        refund the difference.
      </Callout>

      <h2>Read next</h2>
      <ul>
        <li>
          <Link href="/docs/concepts/pick-your-stop">Pick your stop</Link>
        </li>
        <li>
          <Link href="/docs/reference/skus">SKU catalog</Link>
        </li>
        <li>
          <Link href="/docs/reference/api">API reference</Link>
        </li>
      </ul>
    </>
  );
}
