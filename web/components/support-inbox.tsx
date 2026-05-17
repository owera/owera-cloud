"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { Card, CardBody, CardHeader, CardTitle } from "./ui/card";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Badge } from "./ui/badge";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "./ui/dialog";
import { api, ApiClientError } from "@/lib/api-client";
import type { Ticket, TicketDetail, TicketState } from "@/lib/api-client";
import { relativeTime, shortTimestamp } from "@/lib/format";

interface Props {
  initial: Ticket[];
  /** True iff the upstream API was reachable. */
  live: boolean;
}

const STATE_TONE: Record<TicketState, string> = {
  open: "var(--color-state-running)",
  pending: "var(--color-state-queued)",
  resolved: "var(--color-state-succeeded)",
  closed: "var(--color-muted-foreground)",
};

export function SupportInbox({ initial, live }: Props) {
  const router = useRouter();
  const [tickets, setTickets] = React.useState<Ticket[]>(initial);
  const [selectedId, setSelectedId] = React.useState<string | null>(
    initial.find((t) => t.state === "open")?.id ?? initial[0]?.id ?? null,
  );
  const [detail, setDetail] = React.useState<TicketDetail | null>(null);
  const [loadingDetail, setLoadingDetail] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    if (!selectedId || !live) {
      setDetail(null);
      return;
    }
    let cancelled = false;
    setLoadingDetail(true);
    api
      .getTicket(selectedId)
      .then((d) => {
        if (!cancelled) setDetail(d);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setDetail(null);
          setError(err instanceof Error ? err.message : "Load failed");
        }
      })
      .finally(() => {
        if (!cancelled) setLoadingDetail(false);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedId, live]);

  function onCreated(t: Ticket) {
    setTickets((prev) => [t, ...prev]);
    setSelectedId(t.id);
    router.refresh();
  }

  async function postReply(body: string) {
    if (!selectedId) return;
    try {
      await api.postTicketMessage(selectedId, body);
      const refreshed = await api.getTicket(selectedId);
      setDetail(refreshed);
      router.refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Send failed");
    }
  }

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-[18rem_1fr]">
      <Card>
        <CardHeader className="flex items-center justify-between">
          <CardTitle>TICKETS</CardTitle>
          <NewTicketDialog onCreated={onCreated} />
        </CardHeader>
        <ul className="divide-y divide-[var(--color-border)]">
          {tickets.length === 0 && (
            <li className="px-4 py-6 text-center text-xs text-[var(--color-muted-foreground)]">
              No tickets yet.
            </li>
          )}
          {tickets.map((t) => {
            const on = t.id === selectedId;
            return (
              <li key={t.id}>
                <button
                  type="button"
                  onClick={() => setSelectedId(t.id)}
                  className={
                    "block w-full text-left px-4 py-2.5 transition-colors " +
                    (on
                      ? "bg-[var(--color-muted)]"
                      : "hover:bg-[var(--color-muted)]/40")
                  }
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-xs truncate">
                      {t.subject || "(no subject)"}
                    </span>
                    <Badge tone={STATE_TONE[t.state]}>{t.state}</Badge>
                  </div>
                  <div className="mt-1 flex items-center justify-between text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)] font-mono">
                    <span title={t.updatedAt}>{relativeTime(t.updatedAt)}</span>
                    <span>last: {t.lastAuthor}</span>
                  </div>
                </button>
              </li>
            );
          })}
        </ul>
      </Card>

      <Card>
        {!selectedId && (
          <CardBody className="text-sm text-[var(--color-muted-foreground)]">
            Select a ticket to view the thread.
          </CardBody>
        )}
        {selectedId && (
          <TicketView
            ticketId={selectedId}
            detail={detail}
            loading={loadingDetail}
            live={live}
            error={error}
            onClearError={() => setError(null)}
            onReply={postReply}
            fallback={tickets.find((t) => t.id === selectedId) ?? null}
          />
        )}
      </Card>
    </div>
  );
}

function TicketView({
  ticketId,
  detail,
  loading,
  live,
  error,
  onClearError,
  onReply,
  fallback,
}: {
  ticketId: string;
  detail: TicketDetail | null;
  loading: boolean;
  live: boolean;
  error: string | null;
  onClearError: () => void;
  onReply: (body: string) => Promise<void>;
  fallback: Ticket | null;
}) {
  const [reply, setReply] = React.useState("");
  const [sending, setSending] = React.useState(false);
  void ticketId;

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (sending || !reply.trim()) return;
    setSending(true);
    onClearError();
    await onReply(reply.trim());
    setReply("");
    setSending(false);
  }

  const subject = detail?.subject ?? fallback?.subject ?? "(loading…)";
  const state = detail?.state ?? fallback?.state ?? "open";
  const messages = detail?.messages ?? [];

  return (
    <>
      <CardHeader className="flex items-center justify-between gap-2">
        <CardTitle>{subject}</CardTitle>
        <div className="flex items-center gap-2">
          {!live && (
            <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
              FIXTURE
            </span>
          )}
          <Badge tone={STATE_TONE[state]}>{state}</Badge>
        </div>
      </CardHeader>
      <CardBody className="space-y-3">
        {loading && (
          <div className="text-xs text-[var(--color-muted-foreground)]">
            Loading…
          </div>
        )}
        {error && (
          <div className="text-xs text-[var(--color-state-failed)] font-mono">
            {error}
          </div>
        )}
        {!loading && messages.length === 0 && (
          <div className="text-xs text-[var(--color-muted-foreground)]">
            No messages yet on this ticket.
          </div>
        )}
        <ol className="space-y-3">
          {messages.map((m) => (
            <li
              key={m.id}
              className={
                "rounded border p-3 " +
                (m.author === "support"
                  ? "border-[var(--color-primary)]/40 bg-[var(--color-primary)]/5"
                  : "border-[var(--color-border)]")
              }
            >
              <div className="flex items-center justify-between text-[10px] uppercase tracking-wide font-mono text-[var(--color-muted-foreground)] mb-1">
                <span>{m.author === "support" ? "support" : "you"}</span>
                <span>{shortTimestamp(m.ts)}</span>
              </div>
              <div className="text-sm whitespace-pre-wrap">{m.body}</div>
            </li>
          ))}
        </ol>

        {state !== "closed" && (
          <form
            onSubmit={onSubmit}
            className="space-y-2 border-t border-[var(--color-border)] pt-3"
          >
            <textarea
              value={reply}
              onChange={(e) => setReply(e.target.value)}
              placeholder="Type your reply…"
              rows={3}
              className="w-full rounded-md border bg-[var(--color-input)] border-[var(--color-border)] p-3 text-sm font-mono text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]"
            />
            <div className="flex justify-end">
              <Button
                type="submit"
                variant="primary"
                disabled={sending || !reply.trim()}
              >
                {sending ? "Sending…" : "Send"}
              </Button>
            </div>
          </form>
        )}
      </CardBody>
    </>
  );
}

function NewTicketDialog({
  onCreated,
}: {
  onCreated: (t: Ticket) => void;
}) {
  const [open, setOpen] = React.useState(false);
  const [subject, setSubject] = React.useState("");
  const [body, setBody] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  function reset() {
    setSubject("");
    setBody("");
    setBusy(false);
    setError(null);
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const created = await api.createTicket({ subject, body });
      onCreated(created);
      setOpen(false);
      reset();
    } catch (err) {
      if (err instanceof ApiClientError) {
        setError(`${err.code}: ${err.message}`);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError("Create failed");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        setOpen(next);
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm" variant="primary">
          New
        </Button>
      </DialogTrigger>
      <DialogContent>
        <form onSubmit={onSubmit}>
          <DialogHeader>
            <DialogTitle>OPEN A TICKET</DialogTitle>
            <DialogDescription>
              We respond during business hours (UTC-3). Urgent production
              incidents should also page via the runbook in
              <span className="font-mono"> docs/runbook-deploy.md</span>.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <label className="block">
              <div className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)] mb-1">
                SUBJECT
              </div>
              <Input
                required
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
                placeholder="e.g. job_01HXR8M2 stuck in running"
              />
            </label>
            <label className="block">
              <div className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)] mb-1">
                DESCRIPTION
              </div>
              <textarea
                required
                value={body}
                onChange={(e) => setBody(e.target.value)}
                rows={5}
                className="w-full rounded-md border bg-[var(--color-input)] border-[var(--color-border)] p-3 text-sm font-mono text-[var(--color-foreground)] placeholder:text-[var(--color-muted-foreground)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]"
                placeholder="What happened? What did you expect?"
              />
            </label>
            {error && (
              <div className="text-xs text-[var(--color-state-failed)] font-mono">
                {error}
              </div>
            )}
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="ghost">
                Cancel
              </Button>
            </DialogClose>
            <Button
              type="submit"
              variant="primary"
              disabled={busy || !subject.trim() || !body.trim()}
            >
              {busy ? "Sending…" : "Open ticket"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
