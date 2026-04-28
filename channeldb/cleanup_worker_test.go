package channeldb

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// closeWorkerEventuallyTimeout bounds how long a test waits for the
// asynchronous cleanup worker to drain its queue. The drain itself runs in
// transactions that are sub-100ms each on a typical test backend; allowing
// several seconds keeps these tests stable on slow CI machines without
// hiding genuine bugs.
const closeWorkerEventuallyTimeout = 10 * time.Second

// closeWorkerEventuallyTick is the polling interval used while waiting for
// the worker to drain. Short enough that the test does not noticeably
// delay drain-time observations, long enough not to spam the backend.
const closeWorkerEventuallyTick = 25 * time.Millisecond

// requireQueueDrained polls fetchChannelsPendingCleanup until the queue is
// empty or the timeout elapses. The queue is the sole source of truth for
// pending cleanup work, so its emptiness is the cleanest signal that the
// worker has finished a particular set of entries.
func requireQueueDrained(t *testing.T, cdb *ChannelStateDB) {
	t.Helper()

	require.Eventually(t, func() bool {
		pending, err := cdb.fetchChannelsPendingCleanup(t.Context())
		require.NoError(t, err)

		return len(pending) == 0
	}, closeWorkerEventuallyTimeout, closeWorkerEventuallyTick,
		"cleanup worker did not drain pendingChanCleanupBucket")
}

// TestCleanupWorkerDrainsLeftoversOnStart verifies the crash-recovery path:
// a Phase 1 record persisted by a prior run (here simulated by closing a
// channel before the worker is started) is drained by the worker's initial
// drain pass on the next Start. After Start returns and the queue empties,
// the channel bucket and its revocation-log entries must be gone.
func TestCleanupWorkerDrainsLeftoversOnStart(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("cleanup worker only runs on deferred-cleanup backends")
	}

	ch := createTestChannel(t, cdb, openChannelOption())

	const numRevlogEntries = 8
	writeTestRevlogEntries(t, ch, numRevlogEntries)
	writeTestForwardingPackages(t, ch, 3)

	// Close before Start so the cleanup record sits in the bucket like a
	// leftover from a previous node run that did not finish its drain.
	closeChannelForTest(t, cdb, ch)

	// The queue must already contain the entry.
	pending, err := cdb.fetchChannelsPendingCleanup(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)

	// Start launches the worker, which performs the initial drain pass
	// asynchronously. Stop is registered immediately so a test failure
	// after Start cannot leak the goroutine.
	require.NoError(t, cdb.Start(t.Context()))
	t.Cleanup(func() {
		require.NoError(t, cdb.Stop())
	})

	requireQueueDrained(t, cdb)

	require.Equal(t, -1, countRevlogEntries(t, ch),
		"channel bucket must be deleted by the worker drain")
}

// TestCleanupWorkerDrainsOnPokeAfterClose verifies the runtime path:
// CloseChannel commits Phase 1 and pokes the already-running worker, which
// drains the new entry without waiting for the next Start. This is the
// behaviour change that makes a long-lived node's cleanup happen during
// normal operation rather than at restart.
func TestCleanupWorkerDrainsOnPokeAfterClose(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("cleanup worker only runs on deferred-cleanup backends")
	}

	require.NoError(t, cdb.Start(t.Context()))
	t.Cleanup(func() {
		require.NoError(t, cdb.Stop())
	})

	ch := createTestChannel(t, cdb, openChannelOption())

	const numRevlogEntries = 8
	writeTestRevlogEntries(t, ch, numRevlogEntries)
	writeTestForwardingPackages(t, ch, 3)

	closeChannelForTest(t, cdb, ch)

	requireQueueDrained(t, cdb)

	require.Equal(t, -1, countRevlogEntries(t, ch),
		"channel bucket must be deleted by the worker drain")
}

// TestCleanupWorkerHandlesConcurrentCloses verifies that many channel closes
// fired in parallel all eventually drain. Internally the worker serialises
// purges through a single goroutine — concurrent CloseChannel callers each
// commit Phase 1 atomically and poke the worker, but only one purge runs
// at a time. The test does not assert single-purger explicitly (that would
// require backend instrumentation); it asserts the externally visible
// guarantee: every queued entry eventually completes.
func TestCleanupWorkerHandlesConcurrentCloses(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("cleanup worker only runs on deferred-cleanup backends")
	}

	require.NoError(t, cdb.Start(t.Context()))
	t.Cleanup(func() {
		require.NoError(t, cdb.Stop())
	})

	const numChannels = 6
	channels := make([]*OpenChannel, numChannels)
	for i := range numChannels {
		channels[i] = createTestChannel(t, cdb, openChannelOption())
		writeTestRevlogEntries(t, channels[i], 4)
		writeTestForwardingPackages(t, channels[i], 2)
	}

	// Fire all closes in parallel so their commits and pokes interleave.
	// Phase 1 commits serialise at the backend's write lock; the test
	// only requires that all of them eventually complete.
	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			closeChannelForTest(t, cdb, ch)
		}()
	}
	wg.Wait()

	requireQueueDrained(t, cdb)

	for i, ch := range channels {
		require.Equal(t, -1, countRevlogEntries(t, ch),
			"channel %d bucket must be deleted by the drain", i)
	}
}

// TestCleanupWorkerResumesAfterStop verifies that Stop interrupts the
// worker without losing entries from the durable queue: the bucket entries
// of channels closed before Stop persist across the lifecycle, and a
// subsequent Start drains them. This locks in the design contract that the
// in-memory pokeCh is just a doorbell — losing pokes is harmless because
// canonical state lives in pendingChanCleanupBucket.
func TestCleanupWorkerResumesAfterStop(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("cleanup worker only runs on deferred-cleanup backends")
	}

	ch := createTestChannel(t, cdb, openChannelOption())
	writeTestRevlogEntries(t, ch, 4)
	writeTestForwardingPackages(t, ch, 2)

	// Close while the worker has not yet been started. Phase 1 commits;
	// the in-memory poke goes into a buffer with no reader. Stopping
	// without ever starting must leave the durable record in place.
	closeChannelForTest(t, cdb, ch)
	require.NoError(t, cdb.Stop())

	pending, err := cdb.fetchChannelsPendingCleanup(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1, "stop without start must not eat queue")

	// A subsequent Start drains the leftover.
	require.NoError(t, cdb.Start(t.Context()))
	t.Cleanup(func() {
		require.NoError(t, cdb.Stop())
	})

	requireQueueDrained(t, cdb)
	require.Equal(t, -1, countRevlogEntries(t, ch))
}

// TestCleanupWorkerStopIsIdempotent locks in two safety properties of the
// lifecycle wrapper: Stop without Start does not panic (no goroutine to
// cancel, no work to wait on), and a second Stop after the first does not
// hang. Together these protect against test-helper teardown ordering bugs
// and partial-init failure paths in production wiring.
func TestCleanupWorkerStopIsIdempotent(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("cleanup worker only runs on deferred-cleanup backends")
	}

	require.NoError(t, cdb.Stop(), "stop without start must succeed")
	require.NoError(t, cdb.Start(t.Context()))
	require.NoError(t, cdb.Stop(), "first stop must succeed")
	require.NoError(t, cdb.Stop(), "second stop must succeed")
}
