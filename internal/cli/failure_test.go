package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// A build that fails names the feature, every host it failed to reach once with the command that allows them, and the
// log.
func TestBuildFailureNamesTheStepAndItsCause(t *testing.T) {
	output := `#16 7.237   Temporary failure resolving '` + fixture.Host + `'
#16 54.47 fatal: unable to access 'https://` + fixture.Subdomain + `/go/': Could not resolve host: ` + fixture.Subdomain + `
#16 54.48 Temporary failure resolving '` + fixture.Host + `'
#16 54.48 ERROR: Feature "Go" (` + fixture.Store + `/` + fixture.ThirdFeature + `) failed to install!`

	err := buildFailure(output, fixture.LogFile, errors.New("exit status 1"))

	want := "the cage failed to build at the feature Go (" + fixture.Store + "/" + fixture.ThirdFeature + "): exit status 1\n" +
		"  it could not reach " + fixture.Subdomain + " and " + fixture.Host + "\n" +
		"  allow them with: o3s firewall add " + fixture.Subdomain + " " + fixture.Host + "\n" +
		"  the whole log: " + fixture.LogFile
	// Report a failure named otherwise
	if err.Error() != want {
		t.Errorf("buildFailure() =\n%s\nwant\n%s", err, want)
	}
}

// A build that fails with zero features or hosts to name still names the failure and the log.
func TestBuildFailureNamesAPlainFailure(t *testing.T) {
	err := buildFailure("something else broke", fixture.LogFile, errors.New("exit status 1"))

	// Report a failure named otherwise
	if !strings.HasPrefix(err.Error(), "the cage failed to come up: exit status 1\n") || !strings.Contains(err.Error(), fixture.LogFile) {
		t.Errorf("buildFailure() = %v", err)
	}
}

// A step of the VM that fails names the manager's words, the host this machine failed to reach, and that init goes on
// where it stopped.
func TestVMFailureNamesTheCauseAndTheRetry(t *testing.T) {
	cause := errors.New(`failed to download the image: Get "https://` + fixture.Host + `/image": dial tcp: lookup ` + fixture.Host + ` on 127.0.0.11:53: server misbehaving`)

	err := vmFailure("start the VM", cause, fixture.LogFile)

	want := "start the VM: " + cause.Error() + "\n" +
		"  this machine could not reach " + fixture.Host + ", check its network\n" +
		"  run o3s init again, it goes on where it stopped\n" +
		"  the whole log: " + fixture.LogFile
	// Report a failure named otherwise
	if err.Error() != want {
		t.Errorf("vmFailure() =\n%s\nwant\n%s", err, want)
	}
}
