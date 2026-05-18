import Link from "next/link";
import { CodeBlock } from "@/components/docs/code-block";
import { Callout } from "@/components/docs/callout";

export const metadata = {
  title: "API Reference — POST /api/compose · Owera Docs",
};

export default function ApiReference() {
  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        REFERENCE
      </div>
      <h1>POST /api/compose</h1>
      <p className="lede">
        The agent-programmable surface. Every state the slider can express has a
        URL and a JSON body; both produce the same job.
      </p>

      <h2>Authentication</h2>
      <p>
        Bearer-token auth. Create a key on the{" "}
        <Link href="/api-keys">API keys page</Link>.
      </p>
      <CodeBlock
        lang="bash"
        filename="header"
        code="authorization: Bearer $OWERA_API_KEY"
      />

      <h2>Request body</h2>
      <p>
        Validated server-side against{" "}
        <Link href="/api/compose/schema">/api/compose/schema</Link>{" "}
        (Draft 2020-12). Unknown properties are rejected.
      </p>
      <CodeBlock
        lang="json"
        filename="body"
        code={`{
  "level": "advanced",
  "sku": "campaign-swarm",
  "prompt": "Write a competitive teardown of three vector DBs.",
  "tools": ["web", "code"],
  "budget": { "max_cents": 50, "max_latency_ms": 90000 },
  "idempotency_key": "memo-2026-05-18-001"
}`}
      />

      <h3>Field semantics</h3>
      <table>
        <thead>
          <tr>
            <th>Field</th>
            <th>Type</th>
            <th>Required</th>
            <th>Notes</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>
              <code>level</code>
            </td>
            <td>enum</td>
            <td>yes</td>
            <td>
              <code>simple | standard | advanced | expert | custom</code>.
              Alone, enough to make a valid job — defaults fill the rest.
            </td>
          </tr>
          <tr>
            <td>
              <code>sku</code>
            </td>
            <td>string</td>
            <td>yes</td>
            <td>
              Catalog slug, optionally pinned with <code>@vN</code>.
            </td>
          </tr>
          <tr>
            <td>
              <code>prompt</code>
            </td>
            <td>string</td>
            <td>yes</td>
            <td>1–8000 chars.</td>
          </tr>
          <tr>
            <td>
              <code>tools</code>
            </td>
            <td>string[]</td>
            <td>no</td>
            <td>
              Ignored at <code>level=simple</code>. At higher levels, an
              allowlist of tools the agent may call.
            </td>
          </tr>
          <tr>
            <td>
              <code>budget.max_cents</code>
            </td>
            <td>integer</td>
            <td>no</td>
            <td>Hard cap. Agent stops before exceeding.</td>
          </tr>
          <tr>
            <td>
              <code>budget.max_latency_ms</code>
            </td>
            <td>integer</td>
            <td>no</td>
            <td>Wall-clock cap.</td>
          </tr>
          <tr>
            <td>
              <code>idempotency_key</code>
            </td>
            <td>string</td>
            <td>no</td>
            <td>
              Retry-safe. Submitting the same key twice returns the original
              job.
            </td>
          </tr>
        </tbody>
      </table>

      <h2>Response</h2>
      <p>
        <code>202 Accepted</code> on submit. The job is created and running; poll
        the timeline at <code>/jobs/{`<job_id>`}</code> for state.
      </p>
      <CodeBlock
        lang="json"
        filename="response"
        code={`{
  "job_id": "job_01HF...",
  "status": "submitted",
  "shareUrl": "/jobs/job_01HF...?from=compose&level=advanced",
  "rerunUrl": "/compose?level=advanced&prompt=Write+a+competitive+teardown...",
  "estimate": { "centsLow": 80, "centsHigh": 175, "p50ms": 30000, "p95ms": 90000, "tier": "pro" }
}`}
      />

      <h2>GET /api/compose</h2>
      <p>
        Inspect the surface without submitting. Useful for agents that want to
        see what a state evaluates to before they POST.
      </p>
      <CodeBlock
        lang="bash"
        filename="curl"
        code={`curl 'https://app.owera.ai/api/compose?level=advanced&prompt=hello' \\
  -H 'authorization: Bearer $OWERA_API_KEY'`}
      />

      <h2>Errors</h2>
      <table>
        <thead>
          <tr>
            <th>Status</th>
            <th>Code</th>
            <th>Meaning</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>400</td>
            <td>
              <code>invalid_request</code>
            </td>
            <td>Body failed JSON Schema validation. <code>issues[]</code> details each path.</td>
          </tr>
          <tr>
            <td>401</td>
            <td>
              <code>auth_required</code>
            </td>
            <td>
              The requested <code>level</code> requires sign-in.
            </td>
          </tr>
          <tr>
            <td>402</td>
            <td>
              <code>upgrade_required</code>
            </td>
            <td>The requested level requires a paid plan.</td>
          </tr>
          <tr>
            <td>429</td>
            <td>
              <code>rate_limited</code>
            </td>
            <td>Slow down; see <code>retry-after</code> header.</td>
          </tr>
          <tr>
            <td>5xx</td>
            <td>
              <code>internal_error</code>
            </td>
            <td>
              Open a ticket with the <code>requestId</code> from the response
              envelope.
            </td>
          </tr>
        </tbody>
      </table>

      <Callout kind="trust">
        All errors return the same envelope shape:{" "}
        <code>{`{ code, message, requestId? }`}</code>. Validation failures add{" "}
        <code>issues: [{`{ path, message }`}]</code>.
      </Callout>

      <h2>Discovery</h2>
      <ul>
        <li>
          <Link href="/api/compose/schema">/api/compose/schema</Link> — JSON
          Schema, served as <code>application/schema+json</code>
        </li>
        <li>
          <Link href="/llms.txt">/llms.txt</Link> — every doc page in a
          one-line-per-link index agents can fetch
        </li>
      </ul>
    </>
  );
}
