package model

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
)

// TestMain makes the model package's tests hermetic with respect to DNS.
//
// ValidateFeedURL resolves named hosts through ssrfguard to check them against
// blocked ranges. Under `go test -fuzz` that is a liability: the fuzzer
// synthesizes arbitrary hostnames, each one triggering a real DNS lookup, and a
// slow or unreachable resolver (as in CI) stalls a worker long enough for the
// fuzzing engine to report "fuzzing process hung or terminated unexpectedly" —
// the failures once seen in the weekly Fuzz Testing job for FuzzValidateFeedURL
// and FuzzSanitizeFeedURLs.
//
// Substitute the package resolver (injected into ssrfguard via WithResolver)
// with one whose Dial fails immediately, so every lookup returns at once.
// ssrfguard treats an unresolvable host as allowed (the dial-time guard would
// still catch a rebind), so this matches the existing unit tests, none of which
// assert that a named host resolves to a blocked address. This is scoped to the
// package's own resolver rather than mutating the process-wide
// net.DefaultResolver.
func TestMain(m *testing.M) {
	resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dns lookups are disabled in tests")
		},
	}
	os.Exit(m.Run())
}
