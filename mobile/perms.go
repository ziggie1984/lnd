package lndmobile

// Local permissions manager stub to avoid lightning-terminal/perms dependency
// which has version incompatibilities with lnd v0.20.0

import (
	"gopkg.in/macaroon-bakery.v2/bakery"
)

// PermissionsManager is a local stub for permission management.
// It provides basic URI to permission mapping for LNC functionality.
type PermissionsManager struct {
	perms map[string][]bakery.Op
}

// NewPermissionsManager creates a new permissions manager.
// This is a simplified version that doesn't load permissions from RPC servers.
func NewPermissionsManager() (*PermissionsManager, error) {
	return &PermissionsManager{
		perms: make(map[string][]bakery.Op),
	}, nil
}

// URIPermissions returns the permissions required for a given URI.
// Returns false if the URI is not found in the permissions map.
func (pm *PermissionsManager) URIPermissions(uri string) ([]bakery.Op, bool) {
	ops, ok := pm.perms[uri]
	return ops, ok
}
