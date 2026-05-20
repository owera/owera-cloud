import Link from "next/link";
import { api } from "@/lib/api-client";
import { formatCents } from "@/lib/format";
import type { SKU } from "@/lib/types";

export const metadata = {
  title: "SKU catalog · Owera Docs",
};

export const revalidate = 300;

export default async function SkuCatalog() {
  let skus: SKU[] = [];
  let err: string | null = null;
  try {
    skus = await api.listSkus();
  } catch (e) {
    err = e instanceof Error ? e.message : "Unknown error";
  }

  return (
    <>
      <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
        REFERENCE
      </div>
      <h1>SKU catalog</h1>
      <p className="lede">
        Every published SKU. Pricing here is the <i>base</i> per-unit price the
        cost estimator multiplies by your slider stop&apos;s factor. This page
        revalidates every 5 minutes and is the canonical source for both the
        slider preview and your invoice.
      </p>

      {err && (
        <p className="text-[var(--color-state-failed)] text-sm">
          Could not load SKUs: {err}.
        </p>
      )}

      {skus.length === 0 && !err && (
        <p className="text-[var(--color-muted-foreground)] text-sm">
          No SKUs published yet.
        </p>
      )}

      {skus.length > 0 && (
        <table>
          <thead>
            <tr>
              <th>SKU</th>
              <th>Category</th>
              <th>Unit</th>
              <th>Base price</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {skus.map((s) => (
              <tr key={s.id}>
                <td>
                  <code>{s.slug}</code>
                  <div className="text-[10px] text-[var(--color-muted-foreground)] font-mono">
                    {s.id}
                  </div>
                </td>
                <td>{s.category}</td>
                <td>{s.unit}</td>
                <td>{formatCents(s.unitPriceCents)}</td>
                <td>{s.description || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>Read next</h2>
      <ul>
        <li>
          <Link href="/docs/concepts/cost-and-pricing">
            Cost & pricing model — how base price becomes your bill
          </Link>
        </li>
        <li>
          <Link href="/docs/reference/api">API reference</Link>
        </li>
      </ul>
    </>
  );
}
