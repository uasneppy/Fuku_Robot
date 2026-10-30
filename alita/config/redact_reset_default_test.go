//go:build !testtools

package config

import "testing"

// forgetRegisteredSecrets is a no-op without the testtools tag, which has no registry reset.
func forgetRegisteredSecrets(t *testing.T) { t.Helper() }
