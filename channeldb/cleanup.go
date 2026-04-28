package channeldb

import (
	"context"
	"errors"
	"sync"

	"github.com/lightningnetwork/lnd/kvdb"
)

// purgeBatchSize is the maximum number of bucket entries (keys or sub-
// buckets) deleted per transaction during a batched bucket drain. This bounds
// the duration any single write transaction holds the backend's write lock —
// on KV-over-SQL backends a sub-100ms upper bound is the goal — so other
// writers can interleave between batches when a channel with millions of
// revocation entries is being purged.
const purgeBatchSize = 1000

// errStopIter is a sentinel returned from a kvdb.ForEach callback to break
// out of iteration early without surfacing an error to the caller.
var errStopIter = errors.New("stop iteration")

// drainBucket repeatedly opens a write transaction, deletes up to
// purgeBatchSize entries from the bucket returned by navigate, and commits,
// until the bucket is empty or no longer exists. ctx is checked between
// batches; cancellation does not interrupt an in-flight transaction.
//
// Each batch handles both regular keys and nested sub-buckets: keys are
// deleted via Delete, sub-buckets via DeleteNestedBucket. This lets the
// helper drain both the revocation-log bucket (key-only) and a per-channel
// forwarding-package bucket (nested-bucket-per-height) with the same code.
func (c *ChannelStateDB) drainBucket(ctx context.Context,
	navigate func(kvdb.RwTx) kvdb.RwBucket) error {

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var done bool
		err := kvdb.Update(c.backend, func(tx kvdb.RwTx) error {
			bkt := navigate(tx)
			if bkt == nil {
				done = true
				return nil
			}

			keys := make([][]byte, 0, purgeBatchSize)
			iterErr := bkt.ForEach(func(k, _ []byte) error {
				if len(keys) >= purgeBatchSize {
					return errStopIter
				}
				kCopy := make([]byte, len(k))
				copy(kCopy, k)
				keys = append(keys, kCopy)

				return nil
			})
			if iterErr != nil &&
				!errors.Is(iterErr, errStopIter) {

				return iterErr
			}

			if len(keys) == 0 {
				done = true
				return nil
			}

			for _, k := range keys {
				subBkt := bkt.NestedReadBucket(k)
				if subBkt != nil {
					err := bkt.DeleteNestedBucket(k)
					if err != nil {
						return err
					}

					continue
				}
				if err := bkt.Delete(k); err != nil {
					return err
				}
			}

			return nil
		}, func() {})
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

// cleanupWorker drains pendingChanCleanupBucket asynchronously during normal
// operation. A single goroutine reads the bucket on each wakeup and runs the
// per-channel batched purge for every entry it finds. The bucket itself is
// the durable queue; the worker's pokeCh is just a "go look" doorbell with
// no payload, so the in-memory signal can never disagree with on-disk state.
//
// Single-worker is not just simplicity: the KV-over-SQL backends serialise
// writes at the SQL transaction level, so a second concurrent purger would
// only queue behind the first and add lock-fairness churn. One worker is
// the honest representation of what the backend can do.
type cleanupWorker struct {
	db *ChannelStateDB

	// pokeCh is a size-1 buffered channel used to wake the run loop when
	// new pending cleanup work has been registered. Senders use a
	// non-blocking send and drop on full, since a queued poke is
	// equivalent to "go look at the bucket again" and one queued poke is
	// enough to cover any number of intervening registrations.
	pokeCh chan struct{}

	// cancel cancels the run loop's context. Set by start, called by
	// stop.
	cancel context.CancelFunc

	// wg tracks the run goroutine for orderly shutdown.
	wg sync.WaitGroup
}

// newCleanupWorker constructs a cleanup worker bound to the given store. It
// does not start the run loop; the caller must call start.
func newCleanupWorker(db *ChannelStateDB) *cleanupWorker {
	return &cleanupWorker{
		db:     db,
		pokeCh: make(chan struct{}, 1),
	}
}

// start launches the worker's run loop. The supplied context governs the
// loop's lifetime; calling stop also cancels the loop. start and stop must
// not be called concurrently with each other or with themselves: w.cancel
// is written here without synchronisation, so concurrent callers would
// race on the field. ChannelStateDB.Start/Stop is the only intended caller
// and is invoked sequentially from server.Start.
func (w *cleanupWorker) start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.wg.Add(1)
	go w.run(runCtx)
}

// stop cancels the run loop and waits for it to exit. The worker finishes
// the current batch transaction, if any, before returning. Remaining queue
// entries persist in pendingChanCleanupBucket and resume on the next start.
// stop must not race with start (see start's doc on the unsynchronised
// w.cancel write); a second stop after the first is safe and idempotent.
func (w *cleanupWorker) stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

// poke signals the worker that there may be new cleanup work in the bucket.
// The send is non-blocking; if a poke is already queued, this poke is
// dropped — one queued poke is enough to cover any number of intervening
// registrations because the worker rereads the bucket fresh on every drain.
func (w *cleanupWorker) poke() {
	select {
	case w.pokeCh <- struct{}{}:
	default:
	}
}

// run is the worker's main loop. It performs an initial drain pass to pick
// up any leftovers from prior runs, then waits for pokes (or context
// cancellation) and drains again. Errors are logged; the loop never exits
// on a transient drain error so that subsequent pokes can retry.
func (w *cleanupWorker) run(ctx context.Context) {
	defer w.wg.Done()

	// Initial drain pass picks up any leftovers from a previous run that
	// did not complete its queue before shutdown.
	w.drain(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		case <-w.pokeCh:
			w.drain(ctx)
		}
	}
}

// drain runs the queue drain once, logging any error. A drain is a single
// pass through pendingChanCleanupBucket; entries registered after the pass
// starts are picked up on the next pass triggered by a subsequent poke.
func (w *cleanupWorker) drain(ctx context.Context) {
	if err := w.db.purgeAllPendingClosedChannels(ctx); err != nil {
		// Suppress logging on context cancellation: that is the
		// expected exit path during stop, not an error.
		if ctx.Err() != nil {
			return
		}
		log.Errorf("Closed-channel cleanup drain failed: %v", err)
	}
}
