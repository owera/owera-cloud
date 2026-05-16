import * as React from "react";
import { Card, CardBody, CardFooter, CardHeader, CardTitle } from "./ui/card";
import { Badge } from "./ui/badge";
import type { SKU } from "@/lib/types";
import { formatCents } from "@/lib/format";

export interface SkuCardProps {
  sku: SKU;
}

function priceLabel(sku: SKU): string {
  const base = formatCents(sku.unitPriceCents);
  switch (sku.unit) {
    case "job":
      return `${base} / job`;
    case "minute":
      return `${base} / minute`;
    case "mtokens":
      return `${base} / 1M tokens`;
  }
}

export function SkuCard({ sku }: SkuCardProps) {
  return (
    <Card className="flex flex-col">
      <CardHeader className="flex items-center justify-between gap-2">
        <CardTitle>{sku.slug}</CardTitle>
        <Badge>{sku.category}</Badge>
      </CardHeader>
      <CardBody className="flex-1">
        <div className="font-mono text-sm font-semibold text-[var(--color-foreground)] mb-1">
          {sku.name}
        </div>
        <p className="text-xs text-[var(--color-muted-foreground)] leading-relaxed">
          {sku.description}
        </p>
      </CardBody>
      <CardFooter className="flex items-center justify-between">
        <span className="font-mono">{priceLabel(sku)}</span>
        <span className="text-[10px] uppercase tracking-wide">
          ID {sku.id}
        </span>
      </CardFooter>
    </Card>
  );
}
