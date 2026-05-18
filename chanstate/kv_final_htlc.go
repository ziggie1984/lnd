package chanstate

import (
	"errors"
	"fmt"

	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
)

var (
	// ErrFinalHtlcsBucketNotFound signals that the top-level final htlcs
	// bucket does not exist.
	ErrFinalHtlcsBucketNotFound = errors.New("final htlcs bucket not " +
		"found")

	// ErrFinalChannelBucketNotFound signals that the channel bucket for
	// final htlc outcomes does not exist.
	ErrFinalChannelBucketNotFound = errors.New("final htlcs channel " +
		"bucket not found")

	// ErrHtlcUnknown signals that an htlc has no final resolution yet.
	ErrHtlcUnknown = errors.New("htlc unknown")
)

var (
	// finalHtlcsBucket contains the htlcs that have been resolved
	// definitively. Within this bucket, there is a sub-bucket for each
	// channel. In each channel bucket, the htlc indices are stored along
	// with final outcome.
	//
	// final-htlcs -> chanID -> htlcIndex -> outcome
	//
	// 'outcome' is a byte value that encodes:
	//
	//       | true      false
	// ------+------------------
	// bit 0 | settled   failed
	// bit 1 | offchain  onchain
	//
	// This bucket is positioned at the root level, because its contents
	// will be kept independent of the channel lifecycle. This is to avoid
	// the situation where a channel force-closes autonomously and the user
	// not being able to query for htlc outcomes anymore.
	finalHtlcsBucket = []byte("final-htlcs")
)

// finalHtlcByte defines a byte type that encodes information about the final
// htlc resolution.
type finalHtlcByte byte

const (
	// finalHtlcSettledBit is the bit that encodes whether the htlc was
	// settled or failed.
	finalHtlcSettledBit finalHtlcByte = 1 << 0

	// finalHtlcOffchainBit is the bit that encodes whether the htlc was
	// resolved offchain or onchain.
	finalHtlcOffchainBit finalHtlcByte = 1 << 1
)

// fetchFinalHtlcsBucket returns the read-only final htlc bucket for a channel.
func fetchFinalHtlcsBucket(tx kvdb.RTx,
	chanID lnwire.ShortChannelID) (kvdb.RBucket, error) {

	finalHtlcsBucket := tx.ReadBucket(finalHtlcsBucket)
	if finalHtlcsBucket == nil {
		return nil, ErrFinalHtlcsBucketNotFound
	}

	var chanIDBytes [8]byte
	byteOrder.PutUint64(chanIDBytes[:], chanID.ToUint64())

	chanBucket := finalHtlcsBucket.NestedReadBucket(chanIDBytes[:])
	if chanBucket == nil {
		return nil, ErrFinalChannelBucketNotFound
	}

	return chanBucket, nil
}

// fetchFinalHtlcsBucketRw returns the writable final htlc bucket for a channel.
func fetchFinalHtlcsBucketRw(tx kvdb.RwTx,
	chanID lnwire.ShortChannelID) (kvdb.RwBucket, error) {

	finalHtlcsBucket, err := tx.CreateTopLevelBucket(finalHtlcsBucket)
	if err != nil {
		return nil, err
	}

	var chanIDBytes [8]byte
	byteOrder.PutUint64(chanIDBytes[:], chanID.ToUint64())
	chanBucket, err := finalHtlcsBucket.CreateBucketIfNotExists(
		chanIDBytes[:],
	)
	if err != nil {
		return nil, err
	}

	return chanBucket, nil
}

// putFinalHtlc writes the final htlc outcome to the database. Additionally it
// records whether the htlc was resolved off-chain or on-chain.
func putFinalHtlc(finalHtlcsBucket kvdb.RwBucket, id uint64,
	info FinalHtlcInfo) error {

	var key [8]byte
	byteOrder.PutUint64(key[:], id)

	var finalHtlcByte finalHtlcByte
	if info.Settled {
		finalHtlcByte |= finalHtlcSettledBit
	}
	if info.Offchain {
		finalHtlcByte |= finalHtlcOffchainBit
	}

	return finalHtlcsBucket.Put(key[:], []byte{byte(finalHtlcByte)})
}

// fetchFinalHtlc reads a final htlc outcome from the final htlc channel bucket.
func fetchFinalHtlc(finalHtlcsBucket kvdb.RBucket,
	htlcIndex uint64) (*FinalHtlcInfo, error) {

	var idBytes [8]byte
	byteOrder.PutUint64(idBytes[:], htlcIndex)

	value := finalHtlcsBucket.Get(idBytes[:])
	if value == nil {
		return nil, ErrHtlcUnknown
	}

	if len(value) != 1 {
		return nil, errors.New("unexpected final htlc value length")
	}

	info := FinalHtlcInfo{
		Settled:  value[0]&byte(finalHtlcSettledBit) != 0,
		Offchain: value[0]&byte(finalHtlcOffchainBit) != 0,
	}

	return &info, nil
}

// LookupFinalHtlc retrieves a final htlc resolution from the database. If the
// htlc has no final resolution yet, ErrHtlcUnknown is returned.
func (s *KVStore) LookupFinalHtlc(chanID lnwire.ShortChannelID,
	htlcIndex uint64) (*FinalHtlcInfo, error) {

	var info *FinalHtlcInfo

	err := kvdb.View(s.backend, func(tx kvdb.RTx) error {
		finalHtlcsBucket, err := fetchFinalHtlcsBucket(tx, chanID)
		switch {
		case errors.Is(err, ErrFinalHtlcsBucketNotFound):
			fallthrough

		case errors.Is(err, ErrFinalChannelBucketNotFound):
			return ErrHtlcUnknown

		case err != nil:
			return fmt.Errorf("cannot fetch final htlcs bucket: %w",
				err)
		}

		info, err = fetchFinalHtlc(finalHtlcsBucket, htlcIndex)

		return err
	}, func() {
		info = nil
	})
	if err != nil {
		return nil, err
	}

	return info, nil
}

// PutOnchainFinalHtlcOutcome stores the final on-chain outcome of an htlc in
// the database.
func (s *KVStore) PutOnchainFinalHtlcOutcome(chanID lnwire.ShortChannelID,
	htlcID uint64, settled bool) error {

	// Skip if the user did not opt in to storing final resolutions.
	if !s.storeFinalHtlcResolutions {
		return nil
	}

	return kvdb.Update(s.backend, func(tx kvdb.RwTx) error {
		finalHtlcsBucket, err := fetchFinalHtlcsBucketRw(tx, chanID)
		if err != nil {
			return err
		}

		return putFinalHtlc(
			finalHtlcsBucket, htlcID,
			FinalHtlcInfo{
				Settled:  settled,
				Offchain: false,
			},
		)
	}, func() {})
}

// processFinalHtlc stores a final htlc outcome in the database if signaled via
// the supplied log update. An in-memory htlcs map is updated too.
func processFinalHtlc(finalHtlcsBucket kvdb.RwBucket, upd LogUpdate,
	finalHtlcs map[uint64]bool) error {

	var (
		settled bool
		id      uint64
	)

	switch msg := upd.UpdateMsg.(type) {
	case *lnwire.UpdateFulfillHTLC:
		settled = true
		id = msg.ID

	case *lnwire.UpdateFailHTLC:
		settled = false
		id = msg.ID

	case *lnwire.UpdateFailMalformedHTLC:
		settled = false
		id = msg.ID

	default:
		return nil
	}

	// Store the final resolution in the database if a bucket is provided.
	if finalHtlcsBucket != nil {
		err := putFinalHtlc(
			finalHtlcsBucket, id,
			FinalHtlcInfo{
				Settled:  settled,
				Offchain: true,
			},
		)
		if err != nil {
			return err
		}
	}

	finalHtlcs[id] = settled

	return nil
}
