// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

// Package store holds the persistence port and, beside it in a subdirectory,
// its durable implementation.
//
// The split is the same one internal/media uses and it is here for the same
// reason. This package is the interface and an in-memory implementation of it,
// and it carries no driver, so the orchestration suite can hold a real store
// without a database, a file or a build tag. The durable implementation is in
// internal/store/sqlite, which is the only package in the tree that imports a
// database driver, and nothing but the wiring at the entry point imports that.
//
// What is durable and what is not comes from
// docs/decisions/channel-and-room-model.md rather than from convenience.
// Channels, memberships, permission grants and per-person volume settings
// survive a restart. Occupancy does not, and there is deliberately nothing here
// that would let it: an Occupancy written to a store is a participant list that
// outlives the connections it describes.
//
// Which store, and why, is argued in issue #27.
package store
