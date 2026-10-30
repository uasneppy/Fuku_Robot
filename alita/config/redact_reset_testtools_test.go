//go:build testtools

package config

import (
	"testing"

	"github.com/divkix/Alita_Robot/alita/utils/logredact"
)

// forgetRegisteredSecrets removes the exact-value secrets a test registered when it ends, so
// they do not scrub the logs of later tests. Without the testtools tag it is a no-op.
func forgetRegisteredSecrets(t *testing.T) {
	t.Helper()
	t.Cleanup(logredact.ResetForTest)
}

// TestRegisteredSecretsDoNotOutliveTheTest proves a secret registered inside a test is gone
// from the process-wide redaction registry once that test ends.
func TestRegisteredSecretsDoNotOutliveTheTest(t *testing.T) {
	const secret = "scoped-secret-registered-by-a-subtest"
	t.Run("registers", func(t *testing.T) {
		forgetRegisteredSecrets(t)
		registerSecrets(&Config{BotToken: secret})
		if got := logredact.Scrub("token=" + secret); got == "token="+secret {
			t.Fatalf("the secret was not registered: %q", got)
		}
	})
	if got := logredact.Scrub("token=" + secret); got != "token="+secret {
		t.Errorf("the secret still scrubs after its test ended: %q", got)
	}
}
