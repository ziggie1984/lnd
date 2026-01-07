package lndmobile

// This file contains utility functions adapted from github.com/lightninglabs/chantools
// to avoid version incompatibility issues between chantools and lnd.
// Original source: https://github.com/lightninglabs/chantools (MIT License)

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
)

// ===========================================================================
// Explorer API (from chantools/btc/explorer_api.go)
// ===========================================================================

var (
	ErrTxNotFound = errors.New("transaction not found")
)

type ExplorerAPI struct {
	BaseURL string
}

type TX struct {
	TXID string  `json:"txid"`
	Vin  []*Vin  `json:"vin"`
	Vout []*Vout `json:"vout"`
}

type Vin struct {
	Tixid    string `json:"txid"`
	Vout     int    `json:"vout"`
	Prevout  *Vout  `json:"prevout"`
	Sequence uint32 `json:"sequence"`
}

type Vout struct {
	ScriptPubkey     string `json:"scriptpubkey"`
	ScriptPubkeyAsm  string `json:"scriptpubkey_asm"`
	ScriptPubkeyType string `json:"scriptpubkey_type"`
	ScriptPubkeyAddr string `json:"scriptpubkey_address"`
	Value            uint64 `json:"value"`
	Outspend         *Outspend
}

type Outspend struct {
	Spent  bool      `json:"spent"`
	Txid   string    `json:"txid"`
	Vin    int       `json:"vin"`
	Status *TxStatus `json:"status"`
}

type TxStatus struct {
	Confirmed   bool   `json:"confirmed"`
	BlockHeight int    `json:"block_height"`
	BlockHash   string `json:"block_hash"`
}

type Stats struct {
	FundedTXOCount uint32 `json:"funded_txo_count"`
	FundedTXOSum   uint64 `json:"funded_txo_sum"`
	SpentTXOCount  uint32 `json:"spent_txo_count"`
	SpentTXOSum    uint64 `json:"spent_txo_sum"`
	TXCount        uint32 `json:"tx_count"`
}

type AddressStats struct {
	Address      string `json:"address"`
	ChainStats   *Stats `json:"chain_stats"`
	MempoolStats *Stats `json:"mempool_stats"`
}

func (a *ExplorerAPI) Transaction(txid string) (*TX, error) {
	tx := &TX{}
	err := fetchJSON(fmt.Sprintf("%s/tx/%s", a.BaseURL, txid), tx)
	if err != nil {
		return nil, err
	}
	for idx, vout := range tx.Vout {
		url := fmt.Sprintf(
			"%s/tx/%s/outspend/%d", a.BaseURL, txid, idx,
		)
		outspend := Outspend{}
		err := fetchJSON(url, &outspend)
		if err != nil {
			return nil, err
		}
		vout.Outspend = &outspend
	}
	return tx, nil
}

func (a *ExplorerAPI) Unspent(addr string) ([]*Vout, error) {
	var (
		stats   = &AddressStats{}
		outputs []*Vout
		txs     []*TX
		err     error
	)
	err = fetchJSON(fmt.Sprintf("%s/address/%s", a.BaseURL, addr), &stats)
	if err != nil {
		return nil, err
	}

	confirmedUnspent := stats.ChainStats.FundedTXOSum -
		stats.ChainStats.SpentTXOSum
	unconfirmedUnspent := stats.MempoolStats.FundedTXOSum -
		stats.MempoolStats.SpentTXOSum

	if confirmedUnspent+unconfirmedUnspent == 0 {
		return nil, nil
	}

	err = fetchJSON(fmt.Sprintf("%s/address/%s/txs", a.BaseURL, addr), &txs)
	if err != nil {
		return nil, err
	}
	for _, tx := range txs {
		for voutIdx, vout := range tx.Vout {
			if vout.ScriptPubkeyAddr == addr {
				vout.Outspend = &Outspend{
					Txid: tx.TXID,
					Vin:  voutIdx,
				}
				outputs = append(outputs, vout)
			}
		}
	}

	// Now filter those that are really unspent, because above we get all
	// outputs that are sent to the address.
	var unspent []*Vout
	for _, vout := range outputs {
		url := fmt.Sprintf(
			"%s/tx/%s/outspend/%d", a.BaseURL, vout.Outspend.Txid,
			vout.Outspend.Vin,
		)
		outspend := Outspend{}
		err := fetchJSON(url, &outspend)
		if err != nil {
			return nil, err
		}

		if !outspend.Spent {
			unspent = append(unspent, vout)
		}
	}

	return unspent, nil
}

func (a *ExplorerAPI) PublishTx(rawTxHex string) (string, error) {
	url := a.BaseURL + "/tx"
	resp, err := http.Post(url, "text/plain", strings.NewReader(rawTxHex))
	if err != nil {
		return "", fmt.Errorf("error posting data to API '%s', "+
			"server might be experiencing temporary issues, try "+
			"again later; error details: %w", url, err)
	}
	defer resp.Body.Close()
	body := new(bytes.Buffer)
	_, err = body.ReadFrom(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error fetching data from API '%s', "+
			"server might be experiencing temporary issues, try "+
			"again later; error details: %w", url, err)
	}
	return body.String(), nil
}

func fetchJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error fetching data from API '%s', "+
			"server might be experiencing temporary issues, try "+
			"again later; error details: %w", url, err)
	}
	defer resp.Body.Close()

	body := new(bytes.Buffer)
	_, err = body.ReadFrom(resp.Body)
	if err != nil {
		return fmt.Errorf("error fetching data from API '%s', "+
			"server might be experiencing temporary issues, try "+
			"again later; error details: %w", url, err)
	}
	err = json.Unmarshal(body.Bytes(), target)
	if err != nil {
		if body.String() == "Transaction not found" {
			return ErrTxNotFound
		}

		return fmt.Errorf("error decoding data from API '%s', "+
			"server might be experiencing temporary issues, try "+
			"again later; error details: %w", url, err)
	}

	return nil
}

// ===========================================================================
// HD Keychain utilities (from chantools/lnd/hdkeychain.go)
// ===========================================================================

const (
	HardenedKeyStart = uint32(hdkeychain.HardenedKeyStart)
)

func DeriveChildren(key *hdkeychain.ExtendedKey, path []uint32) (
	*hdkeychain.ExtendedKey, error) {

	var currentKey = key
	for idx, pathPart := range path {
		derivedKey, err := currentKey.DeriveNonStandard(pathPart)
		if err != nil {
			return nil, err
		}

		// There's this special case in lnd's wallet (btcwallet) where
		// the coin type and account keys are always serialized as a
		// string and encrypted, which actually fixes the key padding
		// issue that makes the difference between DeriveNonStandard and
		// Derive. To replicate lnd's behavior exactly, we need to
		// serialize and de-serialize the extended key at the coin type
		// and account level (depth = 2 or depth = 3). This does not
		// apply to the default account (id = 0) because that is always
		// derived directly.
		depth := derivedKey.Depth()
		keyID := pathPart - hdkeychain.HardenedKeyStart
		nextID := uint32(0)
		if depth == 2 && len(path) > 2 {
			nextID = path[idx+1] - hdkeychain.HardenedKeyStart
		}
		if (depth == 2 && nextID != 0) || (depth == 3 && keyID != 0) {
			currentKey, err = hdkeychain.NewKeyFromString(
				derivedKey.String(),
			)
			if err != nil {
				return nil, err
			}
		} else {
			currentKey = derivedKey
		}
	}
	return currentKey, nil
}

func ParsePath(path string) ([]uint32, error) {
	path = strings.TrimSpace(path)
	if len(path) == 0 {
		return nil, errors.New("path cannot be empty")
	}
	if !strings.HasPrefix(path, "m/") {
		return nil, errors.New("path must start with m/")
	}
	parts := strings.Split(path, "/")
	indices := make([]uint32, len(parts)-1)
	for i := 1; i < len(parts); i++ {
		index := uint32(0)
		part := parts[i]
		if strings.Contains(parts[i], "'") {
			index += HardenedKeyStart
			part = strings.TrimRight(parts[i], "'")
		}
		parsed, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("could not parse part \"%s\": "+
				"%v", part, err)
		}
		indices[i-1] = index + uint32(parsed)
	}
	return indices, nil
}

func GetWitnessAddrScript(addr btcutil.Address,
	chainParams *chaincfg.Params) ([]byte, error) {

	if !addr.IsForNet(chainParams) {
		return nil, fmt.Errorf("address %v is not for net %v", addr,
			chainParams.Name)
	}

	return txscript.PayToAddrScript(addr)
}

func P2WKHAddr(pubKey *btcec.PublicKey,
	params *chaincfg.Params) (*btcutil.AddressWitnessPubKeyHash, error) {

	hash160 := btcutil.Hash160(pubKey.SerializeCompressed())
	return btcutil.NewAddressWitnessPubKeyHash(hash160, params)
}

func P2AnchorStaticRemote(pubKey *btcec.PublicKey,
	params *chaincfg.Params) (*btcutil.AddressWitnessScriptHash, []byte,
	error) {

	commitScript, err := input.CommitScriptToRemoteConfirmed(pubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create script: %w", err)
	}
	scriptHash := sha256.Sum256(commitScript)
	p2wsh, err := btcutil.NewAddressWitnessScriptHash(scriptHash[:], params)
	return p2wsh, commitScript, err
}

func P2TaprootStaticRemote(pubKey *btcec.PublicKey,
	params *chaincfg.Params) (*btcutil.AddressTaproot,
	*input.CommitScriptTree, error) {

	scriptTree, err := input.NewRemoteCommitScriptTree(
		pubKey, fn.None[txscript.TapLeaf](),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create script: %w", err)
	}

	addr, err := btcutil.NewAddressTaproot(
		schnorr.SerializePubKey(scriptTree.TaprootKey), params,
	)
	return addr, scriptTree, err
}

func PrepareWalletAddress(addr string, chainParams *chaincfg.Params,
	estimator *input.TxWeightEstimator, rootKey *hdkeychain.ExtendedKey,
	hint string) ([]byte, error) {

	const AddressDeriveFromWallet = "fromseed"

	// We already checked if deriving a new address is allowed in a previous
	// step, so we can just go ahead and do it now if requested.
	if addr == AddressDeriveFromWallet {
		// To maximize compatibility and recoverability, we always
		// derive the very first P2WKH address from the wallet.
		// This corresponds to the derivation path: m/84'/0'/0'/0/0.
		derivedKey, err := DeriveChildren(rootKey, []uint32{
			HardenedKeyStart + waddrmgr.KeyScopeBIP0084.Purpose,
			HardenedKeyStart + chainParams.HDCoinType,
			HardenedKeyStart + 0, 0, 0,
		})
		if err != nil {
			return nil, err
		}

		derivedPubKey, err := derivedKey.ECPubKey()
		if err != nil {
			return nil, err
		}

		p2wkhAddr, err := P2WKHAddr(derivedPubKey, chainParams)
		if err != nil {
			return nil, err
		}

		if estimator != nil {
			estimator.AddP2WKHOutput()
		}

		return txscript.PayToAddrScript(p2wkhAddr)
	}

	// Parse the address.
	parsedAddr, err := btcutil.DecodeAddress(addr, chainParams)
	if err != nil {
		return nil, fmt.Errorf("%s address is invalid: %w", hint, err)
	}

	if !parsedAddr.IsForNet(chainParams) {
		return nil, fmt.Errorf("address: %v is not valid for this "+
			"network: %v", parsedAddr.String(), chainParams.Name)
	}

	// Exit early if we don't need to estimate the weight.
	if estimator == nil {
		return txscript.PayToAddrScript(parsedAddr)
	}

	// These are the three address types that we support in general.
	switch parsedAddr.(type) {
	case *btcutil.AddressWitnessPubKeyHash:
		estimator.AddP2WKHOutput()

	case *btcutil.AddressWitnessScriptHash:
		estimator.AddP2WSHOutput()

	case *btcutil.AddressTaproot:
		estimator.AddP2TROutput()

	default:
		return nil, fmt.Errorf("%s address is of wrong type", hint)
	}

	return txscript.PayToAddrScript(parsedAddr)
}

// ===========================================================================
// Signer (from chantools/lnd/signer.go)
// ===========================================================================

type Signer struct {
	*input.MusigSessionManager

	ExtendedKey *hdkeychain.ExtendedKey
	ChainParams *chaincfg.Params
}

func (s *Signer) SignOutputRaw(tx *wire.MsgTx,
	signDesc *input.SignDescriptor) (input.Signature, error) {

	privKey, err := s.FetchPrivateKey(&signDesc.KeyDesc)
	if err != nil {
		return nil, err
	}

	return SignOutputRawWithPrivateKey(tx, signDesc, privKey)
}

func SignOutputRawWithPrivateKey(tx *wire.MsgTx,
	signDesc *input.SignDescriptor,
	privKey *secp256k1.PrivateKey) (input.Signature, error) {

	witnessScript := signDesc.WitnessScript
	privKey = maybeTweakPrivKey(signDesc, privKey)

	sigHashes := txscript.NewTxSigHashes(tx, signDesc.PrevOutputFetcher)
	if txscript.IsPayToTaproot(signDesc.Output.PkScript) {
		var (
			rawSig []byte
			err    error
		)

		switch signDesc.SignMethod {
		case input.TaprootKeySpendBIP0086SignMethod,
			input.TaprootKeySpendSignMethod:

			rawSig, err = txscript.RawTxInTaprootSignature(
				tx, sigHashes, signDesc.InputIndex,
				signDesc.Output.Value, signDesc.Output.PkScript,
				signDesc.TapTweak, signDesc.HashType,
				privKey,
			)
			if err != nil {
				return nil, err
			}

		case input.TaprootScriptSpendSignMethod:
			leaf := txscript.TapLeaf{
				LeafVersion: txscript.BaseLeafVersion,
				Script:      witnessScript,
			}
			rawSig, err = txscript.RawTxInTapscriptSignature(
				tx, sigHashes, signDesc.InputIndex,
				signDesc.Output.Value, signDesc.Output.PkScript,
				leaf, signDesc.HashType, privKey,
			)
			if err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("unknown sign method: %v",
				signDesc.SignMethod)
		}

		sig, err := schnorr.ParseSignature(
			rawSig[:schnorr.SignatureSize],
		)
		if err != nil {
			return nil, err
		}

		return sig, nil
	}

	amt := signDesc.Output.Value
	sig, err := txscript.RawTxInWitnessSignature(
		tx, sigHashes, signDesc.InputIndex, amt,
		witnessScript, signDesc.HashType, privKey,
	)
	if err != nil {
		return nil, err
	}

	return ecdsa.ParseDERSignature(sig[:len(sig)-1])
}

func (s *Signer) FetchPrivateKey(descriptor *keychain.KeyDescriptor) (
	*btcec.PrivateKey, error) {

	key, err := DeriveChildren(s.ExtendedKey, []uint32{
		HardenedKeyStart + uint32(keychain.BIP0043Purpose),
		HardenedKeyStart + s.ChainParams.HDCoinType,
		HardenedKeyStart + uint32(descriptor.Family),
		0,
		descriptor.Index,
	})
	if err != nil {
		return nil, err
	}
	return key.ECPrivKey()
}

func (s *Signer) ComputeInputScript(_ *wire.MsgTx, _ *input.SignDescriptor) (
	*input.Script, error) {

	return nil, errors.New("unimplemented")
}

func maybeTweakPrivKey(signDesc *input.SignDescriptor,
	privKey *btcec.PrivateKey) *btcec.PrivateKey {

	if len(signDesc.SingleTweak) > 0 {
		return input.TweakPrivKey(privKey, signDesc.SingleTweak)
	}
	return privKey
}
