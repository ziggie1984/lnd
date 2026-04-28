package channeldb

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/wire"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// writeTestRevlogEntries writes n entries into the revocationLogBucket for
// the given channel. It navigates the raw KV tree directly so the test does
// not depend on the higher-level commit-chain machinery.
func writeTestRevlogEntries(t *testing.T, ch *OpenChannel, n int) {
	t.Helper()

	err := kvdb.Update(ch.Db.backend, func(tx kvdb.RwTx) error {
		openChanBkt := tx.ReadWriteBucket(openChannelBucket)
		require.NotNil(t, openChanBkt, "openChannelBucket missing")

		nodePub := ch.IdentityPub.SerializeCompressed()
		nodeBkt := openChanBkt.NestedReadWriteBucket(nodePub)
		require.NotNil(t, nodeBkt, "node bucket missing")

		chainBkt := nodeBkt.NestedReadWriteBucket(ch.ChainHash[:])
		require.NotNil(t, chainBkt, "chain bucket missing")

		var chanKeyBuf bytes.Buffer
		err := graphdb.WriteOutpoint(&chanKeyBuf, &ch.FundingOutpoint)
		require.NoError(t, err)

		chanBkt := chainBkt.NestedReadWriteBucket(chanKeyBuf.Bytes())
		require.NotNil(t, chanBkt, "channel bucket missing")

		logBkt, err := chanBkt.CreateBucketIfNotExists(
			revocationLogBucket,
		)
		require.NoError(t, err)

		for i := range n {
			commit := testChannelCommit
			commit.CommitHeight = uint64(i)

			err := putRevocationLog(logBkt, &commit, 0, 1, false)
			require.NoError(t, err)
		}

		return nil
	}, func() {})
	require.NoError(t, err)
}

// writeTestForwardingPackages writes n empty forwarding packages for the given
// channel using distinct remote commitment heights.
func writeTestForwardingPackages(t *testing.T, ch *OpenChannel, n int) {
	t.Helper()

	packager := NewChannelPackager(ch.ShortChanID())
	err := kvdb.Update(ch.Db.backend, func(tx kvdb.RwTx) error {
		for i := range n {
			pkg := NewFwdPkg(
				ch.ShortChanID(), uint64(i), nil, nil,
			)

			if err := packager.AddFwdPkg(tx, pkg); err != nil {
				return err
			}
		}

		return nil
	}, func() {})
	require.NoError(t, err)
}

// countRevlogEntries returns the number of entries in the revocationLogBucket
// for the given channel, or -1 if the channel bucket no longer exists in
// openChannelBucket (cleanup complete).
func countRevlogEntries(t *testing.T, ch *OpenChannel) int {
	t.Helper()

	count := -1
	err := kvdb.View(ch.Db.backend, func(tx kvdb.RTx) error {
		openChanBkt := tx.ReadBucket(openChannelBucket)
		if openChanBkt == nil {
			return nil
		}

		nodePub := ch.IdentityPub.SerializeCompressed()
		nodeBkt := openChanBkt.NestedReadBucket(nodePub)
		if nodeBkt == nil {
			return nil
		}

		chainBkt := nodeBkt.NestedReadBucket(ch.ChainHash[:])
		if chainBkt == nil {
			return nil
		}

		var chanKeyBuf bytes.Buffer
		if err := graphdb.WriteOutpoint(
			&chanKeyBuf, &ch.FundingOutpoint,
		); err != nil {
			return err
		}

		chanBkt := chainBkt.NestedReadBucket(chanKeyBuf.Bytes())
		if chanBkt == nil {
			return nil
		}

		logBkt := chanBkt.NestedReadBucket(revocationLogBucket)
		if logBkt == nil {
			count = 0
			return nil
		}

		c := 0
		if err := logBkt.ForEach(func(k, v []byte) error {
			c++
			return nil
		}); err != nil {
			return err
		}

		count = c

		return nil
	}, func() {})
	require.NoError(t, err)

	return count
}

// closeChannelForTest invokes CloseChannel on a freshly created OpenChannel,
// using a minimal close summary derived from the channel state itself. The
// helper is shared across close-channel and worker tests to keep call sites
// focused on the scenario being verified rather than on summary construction.
func closeChannelForTest(t *testing.T, cdb *ChannelStateDB, ch *OpenChannel) {
	t.Helper()

	summary := &ChannelCloseSummary{
		ChanPoint:   ch.FundingOutpoint,
		RemotePub:   ch.IdentityPub,
		ChainHash:   ch.ChainHash,
		ShortChanID: ch.ShortChannelID,
		CloseType:   CooperativeClose,
	}
	require.NoError(t, cdb.CloseChannel(ch, summary))
}

// TestCloseChannelPhase1RemovesFromOpenScans verifies that after Phase 1
// (ChannelStateDB.CloseChannel) the closed channel no longer appears in
// any open-channel scan (FetchAllChannels, FetchOpenChannels,
// FetchPermAndTempPeers), and that fetchChannelsPendingCleanup returns the
// outpoint so Phase 2 can clean up the bulk data.
func TestCloseChannelPhase1RemovesFromOpenScans(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	ctx := t.Context()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("pending cleanup only applies to deferred-cleanup " +
			"backends")
	}

	// Create two open channels so we can verify the zombie channel does
	// not contaminate the remaining open one.
	ch1 := createTestChannel(t, cdb, openChannelOption())
	ch2 := createTestChannel(t, cdb, openChannelOption())

	// Write a handful of revlog entries so Phase 2 has something to
	// delete.
	const numRevlogEntries = 5
	writeTestRevlogEntries(t, ch1, numRevlogEntries)

	// Confirm both channels are visible before we close one.
	openChans, err := cdb.FetchAllChannels()
	require.NoError(t, err)
	require.Len(t, openChans, 2)

	// Phase 1: close ch1.
	summary := &ChannelCloseSummary{
		ChanPoint:   ch1.FundingOutpoint,
		RemotePub:   ch1.IdentityPub,
		ChainHash:   ch1.ChainHash,
		CloseType:   CooperativeClose,
		ShortChanID: ch1.ShortChannelID,
	}
	require.NoError(t, cdb.CloseChannel(ch1, summary))

	// After Phase 1 the channel must not appear in any open-channel scan.
	openChans, err = cdb.FetchAllChannels()
	require.NoError(t, err)
	require.Len(t, openChans, 1, "closed channel must not appear")
	require.Equal(
		t, ch2.FundingOutpoint, openChans[0].FundingOutpoint,
	)

	// Both test channels share the same IdentityPub, so after ch1 is
	// closed only ch2 should be returned.
	openChans, err = cdb.FetchOpenChannels(ch1.IdentityPub)
	require.NoError(t, err)
	require.Len(t, openChans, 1)
	require.Equal(
		t, ch2.FundingOutpoint, openChans[0].FundingOutpoint,
	)

	// FetchPermAndTempPeers must not error on the zombie bucket.
	_, err = cdb.FetchPermAndTempPeers(ch1.ChainHash[:])
	require.NoError(t, err)

	// The cleanup task must be registered so a restart can resume it.
	pending, err := cdb.fetchChannelsPendingCleanup(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, ch1.FundingOutpoint, pending[0].ChanPoint)

	// The revlog entries must still be present (Phase 2 not run yet).
	require.Equal(t, numRevlogEntries, countRevlogEntries(t, ch1))

	// The outpoint index entry for ch1 must have flipped from
	// outpointOpen to outpointClosed; ch2's must still be outpointOpen.
	require.Equal(t, outpointClosed, readOutpointStatus(
		t, cdb, ch1.FundingOutpoint,
	))
	require.Equal(t, outpointOpen, readOutpointStatus(
		t, cdb, ch2.FundingOutpoint,
	))
}

// readOutpointStatus decodes the indexStatus TLV byte for the given outpoint
// from outpointBucket. Used by tests to verify the index flip during close.
func readOutpointStatus(t *testing.T, cdb *ChannelStateDB,
	op wire.OutPoint) indexStatus {

	t.Helper()

	var chanKeyBuf bytes.Buffer
	require.NoError(t, graphdb.WriteOutpoint(&chanKeyBuf, &op))

	var status uint8
	err := kvdb.View(cdb.backend, func(tx kvdb.RTx) error {
		bkt := tx.ReadBucket(outpointBucket)
		require.NotNil(t, bkt, "outpointBucket missing")

		raw := bkt.Get(chanKeyBuf.Bytes())
		require.NotNil(t, raw, "outpoint entry missing")

		statusRecord := tlv.MakePrimitiveRecord(
			indexStatusType, &status,
		)
		stream, err := tlv.NewStream(statusRecord)
		if err != nil {
			return err
		}

		return stream.Decode(bytes.NewReader(raw))
	}, func() {
		status = 0
	})
	require.NoError(t, err)

	return indexStatus(status)
}

// TestPhase1HidesChannelFromFetchChannel verifies that after Phase 1, a
// targeted FetchChannel lookup returns ErrChannelNotFound rather than
// surfacing ErrNoChanInfoFound from the half-cleaned-up bucket. This
// exercises the pending-cleanup short-circuit on the channelScanner path,
// which the iteration-based tests do not cover.
func TestPhase1HidesChannelFromFetchChannel(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("pending cleanup only applies to deferred-cleanup " +
			"backends")
	}

	ch := createTestChannel(t, cdb, openChannelOption())

	summary := &ChannelCloseSummary{
		ChanPoint:   ch.FundingOutpoint,
		RemotePub:   ch.IdentityPub,
		ChainHash:   ch.ChainHash,
		ShortChanID: ch.ShortChannelID,
		CloseType:   CooperativeClose,
	}
	require.NoError(t, cdb.CloseChannel(ch, summary))

	_, err = cdb.FetchChannel(ch.FundingOutpoint)
	require.ErrorIs(t, err, ErrChannelNotFound)
}

// TestPhase1HidesChannelFromDirectOpenChannelMethods verifies that direct
// OpenChannel methods using the O(1) channel-bucket helpers preserve the old
// not-found contract after Phase 1 leaves the bucket behind for startup
// cleanup.
func TestPhase1HidesChannelFromDirectOpenChannelMethods(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("pending cleanup only applies to deferred-cleanup " +
			"backends")
	}

	ch := createTestChannel(t, cdb, openChannelOption())

	summary := &ChannelCloseSummary{
		ChanPoint:   ch.FundingOutpoint,
		RemotePub:   ch.IdentityPub,
		ChainHash:   ch.ChainHash,
		ShortChanID: ch.ShortChannelID,
		CloseType:   CooperativeClose,
	}
	require.NoError(t, cdb.CloseChannel(ch, summary))

	require.ErrorIs(t, ch.Refresh(), ErrChannelNotFound)
	require.ErrorIs(t, ch.MarkBorked(), ErrChannelNotFound)
	require.ErrorIs(t, ch.CloseChannel(summary), ErrChannelNotFound)
}

// TestPurgeAllPendingClosedChannelsResumesAfterPartial verifies that if
// Phase 2 only completes for a subset of the queue (simulated by directly
// purging one entry), a follow-up call to PurgeAllPendingClosedChannels
// finishes the remaining entries — i.e. the cleanup state survives across
// drains.
func TestPurgeAllPendingClosedChannelsResumesAfterPartial(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	ctx := t.Context()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("pending cleanup only applies to deferred-cleanup " +
			"backends")
	}

	ch1 := createTestChannel(t, cdb, openChannelOption())
	ch2 := createTestChannel(t, cdb, openChannelOption())

	for _, ch := range []*OpenChannel{ch1, ch2} {
		writeTestRevlogEntries(t, ch, 2)
		writeTestForwardingPackages(t, ch, 1)

		summary := &ChannelCloseSummary{
			ChanPoint:   ch.FundingOutpoint,
			RemotePub:   ch.IdentityPub,
			ChainHash:   ch.ChainHash,
			ShortChanID: ch.ShortChannelID,
			CloseType:   CooperativeClose,
		}
		require.NoError(t, cdb.CloseChannel(ch, summary))
	}

	// Both channels are queued.
	pending, err := cdb.fetchChannelsPendingCleanup(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 2)

	// Drain only the first entry to mimic a crash mid-list.
	require.NoError(t, cdb.purgeClosedChannelData(
		ctx, pending[0].ChanPoint, pending[0].Record,
	))

	// One entry must remain queued.
	pending, err = cdb.fetchChannelsPendingCleanup(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)

	// A re-run finishes the remaining entry and clears the queue.
	require.NoError(t, cdb.purgeAllPendingClosedChannels(ctx))

	pending, err = cdb.fetchChannelsPendingCleanup(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
}

// TestPurgeAllPendingClosedChannels verifies that PurgeAllPendingClosedChannels
// wipes all bulk historical data for a closed channel — the revocation-log
// entries, the forwarding packages, and the channel bucket itself —
// deregisters the pending-cleanup marker, and is idempotent on the empty
// queue.
func TestPurgeAllPendingClosedChannels(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	ctx := t.Context()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("pending cleanup only applies to deferred-cleanup " +
			"backends")
	}

	ch := createTestChannel(t, cdb, openChannelOption())

	const numRevlogEntries = 7
	writeTestRevlogEntries(t, ch, numRevlogEntries)
	writeTestForwardingPackages(t, ch, 5)

	summary := &ChannelCloseSummary{
		ChanPoint: ch.FundingOutpoint,
		RemotePub: ch.IdentityPub,
		ChainHash: ch.ChainHash,
		CloseType: CooperativeClose,
	}
	require.NoError(t, cdb.CloseChannel(ch, summary))

	// Ensure cleanup is registered.
	pending, err := cdb.fetchChannelsPendingCleanup(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)

	err = cdb.purgeAllPendingClosedChannels(ctx)
	require.NoError(t, err)

	// The channel bucket must be gone (countRevlogEntries returns -1).
	require.Equal(t, -1, countRevlogEntries(t, ch),
		"channel bucket must be deleted after purge")

	// Forwarding packages must be gone.
	var fwdPkgs []*FwdPkg
	packager := NewChannelPackager(ch.ShortChanID())
	err = kvdb.View(cdb.backend, func(tx kvdb.RTx) error {
		fwdPkgs, err = packager.LoadFwdPkgs(tx)
		return err
	}, func() {
		fwdPkgs = nil
	})
	require.NoError(t, err)
	require.Empty(t, fwdPkgs)

	// The cleanup task must be deregistered.
	pending, err = cdb.fetchChannelsPendingCleanup(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)

	// Draining again with an empty queue must be idempotent.
	err = cdb.purgeAllPendingClosedChannels(ctx)
	require.NoError(t, err)
}

// TestCloseChannelOneShot exercises the synchronous one-shot close path used
// by backends that do not defer bulk cleanup (bbolt, etcd). It locks in the
// invariant that after CloseChannel returns, the channel bucket and its
// revocation-log entries are already gone — no Phase 2 is required — and the
// pending-cleanup queue stays empty.
func TestCloseChannelOneShot(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	ctx := t.Context()
	if cdb.usesDeferredCloseCleanup() {
		t.Skip("one-shot close only applies to non-deferred backends")
	}

	ch := createTestChannel(t, cdb, openChannelOption())

	const numRevlogEntries = 4
	writeTestRevlogEntries(t, ch, numRevlogEntries)
	writeTestForwardingPackages(t, ch, 3)

	summary := &ChannelCloseSummary{
		ChanPoint:   ch.FundingOutpoint,
		RemotePub:   ch.IdentityPub,
		ChainHash:   ch.ChainHash,
		ShortChanID: ch.ShortChannelID,
		CloseType:   CooperativeClose,
	}
	require.NoError(t, cdb.CloseChannel(ch, summary))

	// The channel bucket and its revocation log must be gone — the
	// synchronous path deletes the bulk data inline.
	require.Equal(t, -1, countRevlogEntries(t, ch),
		"channel bucket must be deleted after one-shot close")

	// Forwarding packages must be gone too.
	var fwdPkgs []*FwdPkg
	packager := NewChannelPackager(ch.ShortChanID())
	err = kvdb.View(cdb.backend, func(tx kvdb.RTx) error {
		fwdPkgs, err = packager.LoadFwdPkgs(tx)
		return err
	}, func() {
		fwdPkgs = nil
	})
	require.NoError(t, err)
	require.Empty(t, fwdPkgs)

	// The pending-cleanup queue must remain empty — the one-shot path
	// does not enqueue anything.
	pending, err := cdb.fetchChannelsPendingCleanup(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
}

// TestFetchChannelsPendingCleanupSkipsMalformed locks in the contract that
// fetchChannelsPendingCleanup logs and skips entries with unparseable keys
// or values rather than returning an error. One bad on-disk record must not
// block cleanup for the rest of the queue, so the function reports only
// well-formed records and leaves operator visibility to the warn-level log.
func TestFetchChannelsPendingCleanupSkipsMalformed(t *testing.T) {
	t.Parallel()

	fullDB, err := MakeTestDB(t)
	require.NoError(t, err)

	cdb := fullDB.ChannelStateDB()
	if !cdb.usesDeferredCloseCleanup() {
		t.Skip("pending cleanup only applies to deferred-cleanup " +
			"backends")
	}

	// Create one valid pending entry by closing a real channel; this
	// also lazily creates pendingChanCleanupBucket.
	ch := createTestChannel(t, cdb, openChannelOption())
	closeChannelForTest(t, cdb, ch)

	// Inject two malformed entries directly into the bucket: one with a
	// key too short to decode as a serialized outpoint, and one with a
	// valid-shaped key but a value that is not a TLV stream.
	err = kvdb.Update(cdb.backend, func(tx kvdb.RwTx) error {
		bkt := tx.ReadWriteBucket(pendingChanCleanupBucket)
		require.NotNil(t, bkt, "pendingChanCleanupBucket missing")

		if err := bkt.Put([]byte("short"), []byte{0x00}); err != nil {
			return err
		}

		var goodKey bytes.Buffer
		op := wire.OutPoint{Index: 9}
		if err := graphdb.WriteOutpoint(&goodKey, &op); err != nil {
			return err
		}

		return bkt.Put(goodKey.Bytes(), []byte("not-a-tlv-stream"))
	}, func() {})
	require.NoError(t, err)

	// Both malformed entries must be silently skipped; only the valid
	// entry from the real Phase 1 close should be returned.
	entries, err := cdb.fetchChannelsPendingCleanup(t.Context())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, ch.FundingOutpoint, entries[0].ChanPoint)
}
