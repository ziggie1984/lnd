package itest

import (
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/stretchr/testify/require"
)

// testFundFeerate tests that we can fund a psbt and specify the feerate
// manually in either sat_per_vbyte or sat_per_kweight.
func testFundFeerate(ht *lntest.HarnessTest) {
	alice := ht.Alice
	expectedFeerateSatPerVByte := uint64(2)

	// Create the funding request with the feerate
	// specified in sat_per_vbyte.
	destAddrResp := alice.RPC.NewAddress(&lnrpc.NewAddressRequest{
		Type: lnrpc.AddressType_WITNESS_PUBKEY_HASH,
	})
	fundReq := &walletrpc.FundPsbtRequest{
		Template: &walletrpc.FundPsbtRequest_Raw{
			Raw: &walletrpc.TxTemplate{
				Outputs: map[string]uint64{
					destAddrResp.Address: uint64(100000),
				},
			},
		},
		Fees: &walletrpc.FundPsbtRequest_SatPerVbyte{
			SatPerVbyte: expectedFeerateSatPerVByte,
		},
	}
	// Fund and Finalize the psbt.
	fundResp := alice.RPC.FundPsbt(fundReq)
	finalizeReq := &walletrpc.FinalizePsbtRequest{
		FundedPsbt: fundResp.FundedPsbt,
	}
	finalizeResp := alice.RPC.FinalizePsbt(finalizeReq)

	// With the PSBT signed, we can broadcast the resulting transaction.
	publishReq := &walletrpc.Transaction{
		TxHex: finalizeResp.RawFinalTx,
	}
	alice.RPC.PublishTransaction(publishReq)

	// We'll mine a block which should include the sweep transaction we
	// generated above.
	block := ht.MineBlocksAndAssertNumTxes(1, 1)[0]
	tx := block.Transactions[1].TxHash()

	// Calculate the feerate of the funding transaction in sat_per_vbyte.
	rawTx := ht.Miner.GetRawTransaction(&tx)
	feerate := ht.CalculateFeeRateSatPerVByte(rawTx.MsgTx())

	// Allow some deviation because weight estimates during tx generation
	// are estimates (ECDSA signature size estimate).
	require.InEpsilon(ht, expectedFeerateSatPerVByte, feerate, 0.01)

	// Test the funding flow with a feerate specified in sat_per_kweight.
	expectedFeerateSatPerKweight := uint64(2500)

	// Create the funding request with the feerate
	// specified in sat_per_kweight.
	fundReq = &walletrpc.FundPsbtRequest{
		Template: &walletrpc.FundPsbtRequest_Raw{
			Raw: &walletrpc.TxTemplate{
				Outputs: map[string]uint64{
					destAddrResp.Address: uint64(100000),
				},
			},
		},
		Fees: &walletrpc.FundPsbtRequest_SatPerKweight{
			SatPerKweight: expectedFeerateSatPerKweight,
		},
	}
	// Fund and Finalize the psbt.
	fundResp = alice.RPC.FundPsbt(fundReq)
	finalizeReq = &walletrpc.FinalizePsbtRequest{
		FundedPsbt: fundResp.FundedPsbt,
	}
	finalizeResp = alice.RPC.FinalizePsbt(finalizeReq)

	// With the PSBT signed, we can broadcast the resulting transaction.
	publishReq = &walletrpc.Transaction{
		TxHex: finalizeResp.RawFinalTx,
	}
	alice.RPC.PublishTransaction(publishReq)

	// We'll mine a block which should include the sweep transaction we
	// generated above.
	block = ht.MineBlocksAndAssertNumTxes(1, 1)[0]

	// The sweep transaction should have exactly one inputs as we only had
	// the single output from above in the wallet.
	tx = block.Transactions[1].TxHash()

	// Calculate the feerate of the funding transaction in sat_per_kweight.
	rawTx = ht.Miner.GetRawTransaction(&tx)
	feerate = ht.CalculateFeeRateSatPerKWeight(rawTx.MsgTx())

	// Allow some deviation because weight estimates during tx generation
	// are estimates (ECDSA signature size estimate).
	require.InEpsilon(ht, expectedFeerateSatPerKweight, feerate, 0.01)
}
