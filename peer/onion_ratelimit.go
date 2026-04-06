package peer

import (
	"errors"

	"github.com/btcsuite/btclog/v2"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/onionmessage"
)

// allowOnionMessage delegates to the IngressLimiter for the
// per-peer-then-global byte-granular rate limit check. A successful
// result wraps fn.Unit; a rejection wraps one of the sentinel errors
// onionmessage.ErrPeerRateLimit or onionmessage.ErrGlobalRateLimit so
// that callers can distinguish the drop reason via errors.Is.
//
// A nil IngressLimiter is treated as "disabled" and always accepts the
// message. This preserves the behavior of test and disabled-onion-
// messaging configurations without forcing callers to construct a real
// limiter.
func allowOnionMessage(limiter onionmessage.IngressLimiter,
	peerKey [33]byte, msgBytes int) fn.Result[fn.Unit] {

	if limiter == nil {
		return fn.Ok(fn.Unit{})
	}

	return limiter.AllowN(peerKey, msgBytes)
}

// logFirstOnionDrop emits a one-shot info log the first time the limiter
// identified by err trips. Per-peer drops go to peerLog (caller's
// peer-prefixed log) so the operator can see which peer first tripped
// the limiter; global drops go to pkgLog (typically the package-level
// peerLog) since they are not attributable to any single peer.
func logFirstOnionDrop(pkgLog, peerLog btclog.Logger, err error,
	limiter onionmessage.IngressLimiter) {

	if limiter == nil {
		return
	}

	switch {
	case errors.Is(err, onionmessage.ErrGlobalRateLimit):
		if limiter.FirstGlobalDropClaim() {
			pkgLog.Infof("onion message global rate limiter " +
				"engaged; further drops logged at trace")
		}

	case errors.Is(err, onionmessage.ErrPeerRateLimit):
		if limiter.FirstPeerDropClaim() {
			peerLog.Infof("onion message per-peer rate limiter " +
				"engaged; further drops logged at trace")
		}
	}
}
