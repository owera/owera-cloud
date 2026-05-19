// Where the job's output lands.
//
// Dashboard is the default and always works. Email/Slack/webhook are
// configured via the dashboard's destinations page (a separate concern) but
// referenced here by name so the composer can offer the choice up front and
// the API can validate it.

export type DeliveryKind = "dashboard" | "email" | "slack" | "webhook";

export interface Delivery {
  kind: DeliveryKind;
  /** For email: address. For slack: channel. For webhook: URL. */
  target?: string;
}

export const DELIVERY_DEFAULTS: Record<DeliveryKind, Delivery> = {
  dashboard: { kind: "dashboard" },
  email: { kind: "email", target: "" },
  slack: { kind: "slack", target: "" },
  webhook: { kind: "webhook", target: "" },
};

export function isDeliveryKind(v: unknown): v is DeliveryKind {
  return v === "dashboard" || v === "email" || v === "slack" || v === "webhook";
}

export function describeDelivery(d: Delivery): string {
  switch (d.kind) {
    case "dashboard":
      return "Deliver to the Owera dashboard (default).";
    case "email":
      return d.target
        ? `Email to ${d.target}.`
        : "Email — recipient to be set.";
    case "slack":
      return d.target
        ? `Post to Slack ${d.target}.`
        : "Post to Slack — channel to be set.";
    case "webhook":
      return d.target
        ? `POST to ${d.target}.`
        : "Webhook — URL to be set.";
  }
}
