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
// ValidateFeedURL delegates to ssrfguard, which resolves named hosts via
// net.LookupIP with no timeout or context. Under `go test -fuzz` that is a
// liability: the fuzzer synthesizes arbitrary hostnames, each one triggering a
// real DNS lookup, and a slow or unreachable resolver (as in CI) stalls a worker
// long enough for the fuzzing engine to report "fuzzing process hung or
// terminated unexpectedly" — the failures seen in the weekly Fuzz Testing job
// for FuzzValidateFeedURL and FuzzSanitizeFeedURLs.
//
// Pointing the default resolver at a dialer that fails immediately makes every
// lookup return at once. ssrfguard treats an unresolvable host as allowed (the
// dial-time guard would still catch a rebind), so this matches the existing unit
// tests, none of which assert that a named host resolves to a blocked address.
func TestMain(m *testing.M) {
	net.DefaultResolver.PreferGo = true
	net.DefaultResolver.Dial = func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("dns lookups are disabled in tests")
	}
	os.Exit(m.Run())
}
