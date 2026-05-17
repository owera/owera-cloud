// Command erasure-worker is the background processor for LGPD Art. 18
// / GDPR Art. 17 right-to-erasure requests. It dequeues from the same
// SQLite-backed queue the api uses for jobs, runs the CompositePurger
// against per-tenant subsystems, and writes start / complete / fail
// audit rows so the deletion is traceable end-to-end.
//
// In production this runs as a separate Fly machine — co-locating it
// with the API would let a busy request loop starve the erasure
// worker.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/erasure"
	"github.com/owera/owera-cloud/api/internal/identity"
	"github.com/owera/owera-cloud/api/internal/queue"
)

func main() {
	dbPath := flag.String("db", "./owera-cloud.db", "SQLite database path (shared with apiserver)")
	poll := flag.Duration("poll", 15*time.Second, "queue poll interval when empty")
	flag.Parse()

	if err := run(*dbPath, *poll); err != nil {
		log.Fatalf("erasure-worker: %v", err)
	}
}

func run(dbPath string, poll time.Duration) error {
	idStore, err := identity.Open(dbPath)
	if err != nil {
		return err
	}
	defer idStore.Close()
	db := idStore.DB()

	auditLog, err := audit.New(db)
	if err != nil {
		return err
	}
	q, err := queue.NewSQLite(db)
	if err != nil {
		return err
	}
	svc, err := erasure.New(db, erasure.AdaptQueue(q), auditLog)
	if err != nil {
		return err
	}

	worker := &erasure.Worker{
		Service:      svc,
		Queue:        erasure.AdaptQueue(q),
		Audit:        auditLog,
		Purger:       &erasure.CompositePurger{DB: db},
		PollInterval: poll,
		ClaimMaxAge:  15 * time.Minute,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("erasure-worker: received %s, draining", sig)
		cancel()
	}()

	log.Printf("erasure-worker: starting; poll=%s db=%s", poll, dbPath)
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
