//go:build testtools

package logredact

// ResetForTest empties the registry of exact-value secrets, so a test that registers some
// (directly or through config loading) does not leave them scrubbing the logs of every
// later test of its process. The structural patterns stay. Only builds with the testtools
// tag have it.
func ResetForTest() {
	reset()
}
