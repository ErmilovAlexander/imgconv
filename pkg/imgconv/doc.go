package imgconv

// Package imgconv exposes a stable public API over the internal image
// conversion engine used by the imgconv CLI.
//
// The package is designed as a production-safe facade:
//   - low-level implementation stays in internal/*
//   - CLI can keep working unchanged
//   - external Go projects can import this package without touching internal/*
