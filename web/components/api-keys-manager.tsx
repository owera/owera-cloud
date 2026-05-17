"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
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
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Badge } from "./ui/badge";
import { Table, THead, TBody, TR, TH, TD } from "./ui/table";
import { api, ApiClientError } from "@/lib/api-client";
import { relativeTime, shortTimestamp } from "@/lib/format";
import type { ApiKey } from "@/lib/types";

interface Props {
  initial: ApiKey[];
  /** True iff the upstream API was reachable when the page rendered. */
  live: boolean;
}

type Scope = ApiKey["scopes"][number];
const ALL_SCOPES: ReadonlyArray<Scope> = [
  "jobs.read",
  "jobs.write",
  "billing.read",
];

export function ApiKeysManager({ initial, live }: Props) {
  const router = useRouter();
  const [keys, setKeys] = React.useState<ApiKey[]>(initial);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [revoking, setRevoking] = React.useState<string | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  async function onRevoke(id: string) {
    setError(null);
    setRevoking(id);
    try {
      await api.revokeApiKey(id);
      setKeys((prev) =>
        prev.map((k) =>
          k.id === id ? { ...k, revokedAt: new Date().toISOString() } : k,
        ),
      );
      router.refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Revoke failed");
    } finally {
      setRevoking(null);
    }
  }

  function onCreated(k: ApiKey) {
    setKeys((prev) => [k, ...prev]);
    router.refresh();
  }

  const active = keys.filter((k) => !k.revokedAt);
  const revoked = keys.filter((k) => k.revokedAt);

  return (
    <div className="space-y-4">
      <header className="flex items-baseline justify-between">
        <div className="flex items-center gap-3">
          {!live && (
            <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
              FIXTURE DATA
            </span>
          )}
          {error && (
            <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-failed)]">
              {error}
            </span>
          )}
        </div>
        <CreateKeyDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          onCreated={onCreated}
        />
      </header>

      <Table>
        <THead>
          <TR>
            <TH>NAME</TH>
            <TH>ID</TH>
            <TH>SECRET</TH>
            <TH>SCOPES</TH>
            <TH>CREATED</TH>
            <TH>LAST USED</TH>
            <TH>ACTION</TH>
          </TR>
        </THead>
        <TBody>
          {active.map((k) => (
            <TR key={k.id}>
              <TD>{k.name}</TD>
              <TD className="text-[var(--color-muted-foreground)]">{k.id}</TD>
              <TD>sk_…{k.lastFour}</TD>
              <TD className="flex gap-1">
                {k.scopes.map((s) => (
                  <Badge key={s}>{s}</Badge>
                ))}
              </TD>
              <TD title={k.createdAt}>{shortTimestamp(k.createdAt)}</TD>
              <TD title={k.lastUsedAt ?? "never"}>
                {k.lastUsedAt ? relativeTime(k.lastUsedAt) : "never"}
              </TD>
              <TD>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={revoking === k.id}
                  onClick={() => onRevoke(k.id)}
                >
                  {revoking === k.id ? "…" : "Revoke"}
                </Button>
              </TD>
            </TR>
          ))}
          {active.length === 0 && (
            <TR>
              <TD colSpan={7} className="text-center text-[var(--color-muted-foreground)] py-6">
                No active keys.
              </TD>
            </TR>
          )}
        </TBody>
      </Table>

      {revoked.length > 0 && (
        <details className="text-xs">
          <summary className="cursor-pointer text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]">
            REVOKED · {revoked.length}
          </summary>
          <Table className="mt-2">
            <TBody>
              {revoked.map((k) => (
                <TR key={k.id}>
                  <TD>{k.name}</TD>
                  <TD className="text-[var(--color-muted-foreground)]">{k.id}</TD>
                  <TD>sk_…{k.lastFour}</TD>
                  <TD
                    className="text-[var(--color-muted-foreground)]"
                    title={k.revokedAt ?? ""}
                  >
                    revoked {k.revokedAt ? relativeTime(k.revokedAt) : ""}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </details>
      )}
    </div>
  );
}

function CreateKeyDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  onCreated: (k: ApiKey) => void;
}) {
  const [name, setName] = React.useState("");
  const [scopes, setScopes] = React.useState<Set<Scope>>(
    () => new Set(["jobs.read", "jobs.write"] as Scope[]),
  );
  const [busy, setBusy] = React.useState(false);
  const [secret, setSecret] = React.useState<string | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  function reset() {
    setName("");
    setScopes(new Set(["jobs.read", "jobs.write"] as Scope[]));
    setBusy(false);
    setSecret(null);
    setError(null);
  }

  function toggleScope(s: Scope) {
    setScopes((prev) => {
      const next = new Set(prev);
      if (next.has(s)) next.delete(s);
      else next.add(s);
      return next;
    });
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const created = await api.createApiKey({
        name: name.trim(),
        scopes: Array.from(scopes),
      });
      setSecret(created.secret);
      const { secret: _omit, ...meta } = created;
      void _omit;
      onCreated(meta);
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

  async function copySecret() {
    if (!secret) return;
    try {
      await navigator.clipboard.writeText(secret);
    } catch {
      // Clipboard may be unavailable (e.g. http://localhost in some browsers);
      // the user can still select+copy from the textbox.
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogTrigger asChild>
        <Button variant="primary">New key</Button>
      </DialogTrigger>
      <DialogContent>
        {secret ? (
          <>
            <DialogHeader>
              <DialogTitle>NEW KEY · COPY NOW</DialogTitle>
              <DialogDescription>
                This is the only time the secret will be shown. Save it in a
                password manager or your CI secret store. Refusing to display it
                again is by design.
              </DialogDescription>
            </DialogHeader>
            <div className="rounded border border-[var(--color-state-running)]/40 bg-[var(--color-state-running)]/10 p-3">
              <code className="font-mono text-xs break-all select-all text-[var(--color-foreground)]">
                {secret}
              </code>
            </div>
            <DialogFooter>
              <Button variant="secondary" onClick={copySecret}>
                Copy
              </Button>
              <DialogClose asChild>
                <Button variant="primary">I’ve saved it</Button>
              </DialogClose>
            </DialogFooter>
          </>
        ) : (
          <form onSubmit={onSubmit}>
            <DialogHeader>
              <DialogTitle>NEW API KEY</DialogTitle>
              <DialogDescription>
                Scoped credential for the public API. The secret is shown
                exactly once.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-3">
              <label className="block">
                <div className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)] mb-1">
                  NAME
                </div>
                <Input
                  required
                  autoFocus
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. ci-runner"
                />
              </label>
              <div>
                <div className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)] mb-1">
                  SCOPES
                </div>
                <div className="flex flex-wrap gap-2">
                  {ALL_SCOPES.map((s) => {
                    const on = scopes.has(s);
                    return (
                      <button
                        key={s}
                        type="button"
                        onClick={() => toggleScope(s)}
                        className={
                          "rounded border px-2 py-1 text-xs font-mono uppercase tracking-wide transition-colors " +
                          (on
                            ? "border-[var(--color-primary)] text-[var(--color-primary)] bg-[var(--color-primary)]/10"
                            : "border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]")
                        }
                      >
                        {s}
                      </button>
                    );
                  })}
                </div>
              </div>
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
                disabled={busy || !name.trim() || scopes.size === 0}
              >
                {busy ? "Creating…" : "Create"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
