package channeldb

import (
	"context"
	"errors"

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
