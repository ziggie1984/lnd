package chanstate

import (
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
)

// FinalHtlcByte is retained while channeldb compatibility wrappers still
// expose final HTLC encoding details.
type FinalHtlcByte = finalHtlcByte

const (
	// FinalHtlcSettledBit identifies a settled final HTLC outcome.
	FinalHtlcSettledBit = finalHtlcSettledBit

	// FinalHtlcOffchainBit identifies an off-chain final HTLC outcome.
	FinalHtlcOffchainBit = finalHtlcOffchainBit
)

// FetchFinalHtlcsBucket returns the final HTLC bucket for a channel.
func FetchFinalHtlcsBucket(tx kvdb.RTx,
	chanID lnwire.ShortChannelID) (kvdb.RBucket, error) {

	return fetchFinalHtlcsBucket(tx, chanID)
}

// FetchFinalHtlcsBucketRw returns the writable final HTLC bucket for a
// channel.
func FetchFinalHtlcsBucketRw(tx kvdb.RwTx,
	chanID lnwire.ShortChannelID) (kvdb.RwBucket, error) {

	return fetchFinalHtlcsBucketRw(tx, chanID)
}

// PutFinalHtlc stores a final HTLC outcome.
func PutFinalHtlc(bucket kvdb.RwBucket, id uint64,
	info FinalHtlcInfo) error {

	return putFinalHtlc(bucket, id, info)
}

// FetchFinalHtlc reads a final HTLC outcome.
func FetchFinalHtlc(bucket kvdb.RBucket,
	htlcIndex uint64) (*FinalHtlcInfo, error) {

	return fetchFinalHtlc(bucket, htlcIndex)
}
