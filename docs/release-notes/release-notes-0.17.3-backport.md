# Release Notes
- [Bug Fixes](#bug-fixes)
- [New Features](#new-features)
  - [Functional Enhancements](#functional-enhancements)
  - [RPC Additions](#rpc-additions)
  - [lncli Additions](#lncli-additions)
- [Improvements](#improvements)
  - [Functional Updates](#functional-updates)
  - [RPC Updates](#rpc-updates)
  - [lncli Updates](#lncli-updates)
  - [Code Health](#code-health)
  - [Breaking Changes](#breaking-changes)
  - [Performance Improvements](#performance-improvements)
  - [Misc](#misc)
- [Technical and Architectural Updates](#technical-and-architectural-updates)
  - [BOLT Spec Updates](#bolt-spec-updates)
  - [Testing](#testing)
  - [Database](#database)
  - [Code Health](#code-health-1)
  - [Tooling and Documentation](#tooling-and-documentation)
- [Contributors (Alphabetical Order)](#contributors-alphabetical-order)


# Bug Fixes
  
* [Fixed](https://github.com/lightningnetwork/lnd/pull/8096) a case where `lnd`
  might dip below its channel reserve when htlcs are added concurrently. A
  fee buffer (additional balance) is now always kept on the local side ONLY
  if the channel was opened locally. This is in accordance with the BOTL 02
  specification and protects against sharp fee changes because there is always
  this buffer which can be used to increase the commitment fee and it also
  protects against the case where htlcs are added asynchronously resulting in
  stuck channels.



  # Contributors (Alphabetical Order)

* Ziggie