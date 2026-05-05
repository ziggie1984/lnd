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
    - [Breaking Changes](#breaking-changes)
    - [Performance Improvements](#performance-improvements)
    - [Deprecations](#deprecations)
- [Technical and Architectural Updates](#technical-and-architectural-updates)
    - [BOLT Spec Updates](#bolt-spec-updates)
    - [Testing](#testing)
    - [Database](#database)
    - [Code Health](#code-health)
    - [Tooling and Documentation](#tooling-and-documentation)
- [Contributors (Alphabetical Order)](#contributors-alphabetical-order)

# Bug Fixes

# New Features

## Functional Enhancements

## RPC Additions

## lncli Additions

# Improvements

## Functional Updates

## RPC Updates

## lncli Updates

## Breaking Changes

## Performance Improvements

## Deprecations

# Technical and Architectural Updates

## BOLT Spec Updates

## Testing

## Database

* Native SQL migration of the channel state and switch data:
  * Introduce the [`chanstate.Store`
    interface](https://github.com/lightningnetwork/lnd/pull/10777) as the
    persistence contract for the channel-state subsystem, decoupling
    consumers from `*channeldb.ChannelStateDB` ahead of the SQL migration.
  * Switch channel-state consumers (`server`, `lnrpc`, `funding`, `peer`,
    `contractcourt`, `channelnotifier`, and the channel restore path)
    over to the [`chanstate.Store`
    interface](https://github.com/lightningnetwork/lnd/pull/10790),
    replacing direct `*channeldb.ChannelStateDB` dependencies so the
    upcoming KV/SQL store implementations can be swapped in transparently.

## Code Health

## Tooling and Documentation

# Contributors (Alphabetical Order)
