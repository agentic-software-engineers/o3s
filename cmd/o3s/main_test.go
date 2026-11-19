package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/registry/registrytest"
)

// update rewrites the expected files inside the scripts to what the binary now writes.
var update = flag.Bool("update", false, "rewrite the expected files inside the scripts")

// fakeDocker stands in for the container engine, so a script sees which restarts a change asks for and what the gateway recorded.
const fakeDocker = `#!/bin/sh
echo "$*" >> "$WORK/docker.log"
# Answer each question about the stack, any restart, and the daemon's security options, the way a host's engine would
case "$*" in
  ps*service=cage*) [ -f "$WORK/cage-up" ] && echo cage-id ;;
  ps*service=gateway*) [ -f "$WORK/stack-up" ] && echo gateway-id ;;
  ps*service=proxy*) [ -f "$WORK/stack-up" ] && echo proxy-id ;;
  restart*) echo "$2" ;;
  info*) [ -f "$WORK/no-apparmor" ] && echo '["name=seccomp,profile=builtin","name=cgroupns"]' || echo '["name=apparmor","name=seccomp,profile=builtin","name=cgroupns"]' ;;
  "exec gateway-id cat /state/traffic.json") [ -f "$WORK/traffic.json" ] && cat "$WORK/traffic.json" || exit 1 ;;
esac
exit 0
`

// fakeDevcontainer stands in for the devcontainer CLI, so a script sees each rebuild an update asks for and each command run in the cage.
const fakeDevcontainer = `#!/bin/sh
echo "$*" >> "$WORK/devcontainer.log"
# Answer each command in the cage the way the cage would, as the files a script leaves in $WORK say
case "$*" in
  up*) echo '{"outcome":"success","containerId":"cage-id","composeProjectName":"o3s"}' ;;
  exec*"ssh-add -l") [ -f "$WORK/no-agent" ] && exit 2 ;;
  exec*"ls -A "*) [ -f "$WORK/not-empty" ] && echo README.md ;;
  exec*"git clone "*) [ -f "$WORK/clone-fails" ] && { echo "fatal: repository not found" >&2; exit 128; } ;;
esac
exit 0
`

// fakeLimactl stands in for Lima, keeping its VMs in a file, so a script sees each VM o3s boots, runs commands in and
// deletes. Like Lima, it writes its log records as JSON on stderr and a download's progress as plain lines, and it fails
// with a fatal record where $WORK/lima-fails holds the message.
const fakeLimactl = `#!/bin/sh
echo "LIMA_HOME=$LIMA_HOME $*" >> "$WORK/limactl.log"
# Read past the global flags to the command, as Lima does
while [ "${1#--}" != "$1" ]; do shift; done
# Fail with the record a test asks for
if [ -f "$WORK/lima-fails" ] && [ "$1" = start ]; then
  printf '{"level":"fatal","msg":"%s"}\n' "$(cat "$WORK/lima-fails")" >&2
  exit 1
fi
# Answer each command the way Lima does, with the VMs listed in $WORK/vms
case "$1" in
  list) case "$2" in --format=*) grep -qx "$3" "$WORK/vms" || exit 1; grep -qx "$3" "$WORK/stopped" 2>/dev/null && echo Stopped || echo Running ;; *) cat "$WORK/vms" 2>/dev/null || true ;; esac ;;
  stop) echo "$2" >> "$WORK/stopped" ;;
  start) rm -f "$WORK/stopped"; echo '{"level":"info","msg":"Starting the instance"}' >&2
    echo '819.29 MiB / 819.29 MiB [--] 100.00%' >&2
    for a in "$@"; do case "$a" in --name=*) echo "${a#--name=}" >> "$WORK/vms" ;; -) cat > "$WORK/template.yaml" ;; esac; done ;;
  delete) grep -v "$3" "$WORK/vms" > "$WORK/vms.left"; mv "$WORK/vms.left" "$WORK/vms" ;;
  shell) grep -qx "$2" "$WORK/vms" 2>/dev/null || { echo '{"level":"fatal","msg":"instance does not exist"}' >&2; exit 1; }; shift 2; echo "$*" >> "$WORK/vm.log"
    # Leave the installer and the home's setup to the log, and run any other command here
    case "$*" in *install.sh*|*"init --guest"*) ;; *) "$@" ;; esac ;;
esac
`

// fakeQEMU stands in for QEMU's tools, which a Linux host runs the VM with.
const fakeQEMU = "#!/bin/sh\n"

// fakeCode stands in for the VS Code CLI, so a script sees what it opens.
const fakeCode = `#!/bin/sh
echo "$*" >> "$WORK/code.log"
`

// fakeWslpath stands in for WSL's path converter, naming a folder's Windows form and the Windows side's APPDATA.
const fakeWslpath = `#!/bin/sh
echo "$*" >> "$WORK/wslpath.log"
# Convert the way WSL does, into the Windows form or back into the Linux one
case "$1" in
  -w) echo "\\\\wsl.localhost\\Ubuntu$2" ;;
  -u) echo "$WORK/appdata" ;;
esac
`

// fakeCmd stands in for the Windows shell, naming the APPDATA folder.
const fakeCmd = `#!/bin/sh
echo 'C:\Users\ubuntu\AppData\Roaming'
`

// fakeAaEnabled answers as a host whose kernel runs no AppArmor, so no script asks for sudo.
const fakeAaEnabled = `#!/bin/sh
exit 1
`

// tool is a feature that needs a host with a secret, sets env, and keeps its data in a state dir.
const tool = `{
  "id": "tool",
  "version": "2.0.0",
  "options": {
    "stateDir": {"type": "string", "default": ""},
    "mode": {"type": "string", "enum": ["strict", "loose"], "default": "strict", "description": "How strictly the tool checks"},
    "debug": {"type": "boolean", "default": false, "description": "Log every step"},
    "version": {"type": "string", "proposals": ["latest", "lts"], "default": "latest"}
  },
  "customizations": {"o3s": {
    "hosts": [{"name": "example.com", "ports": [443], "secret": "EXAMPLE_TOKEN", "description": "API"}],
    "env": [
      {"name": "EXAMPLE_MODE", "value": "strict", "description": "Mode"},
      {"name": "EXAMPLE_HOME", "value": "${stateDir}", "description": "Data"}
    ]
  }}
}`

// TestMain lets the scripts run this binary as o3s.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"o3s": main})
}

// Every scenario in the scripts holds for the o3s binary.
func TestScripts(t *testing.T) {
	reg, err := registrytest.New()
	// Stop when the registry fails to come up
	if err != nil {
		t.Fatalf("Setup: registrytest.New() = %v", err)
	}
	t.Cleanup(reg.Close)
	// go installs after node, as its store tells
	goAfterNode := fmt.Sprintf(`{"id": "go", "version": "1.0.0", "options": {"version": {"type": "string", "default": "latest"}}, "installsAfter": [%q]}`, reg.Host+"/features/node")
	// Stop when the store fails to publish
	if err := reg.PushFeatures("features", `{"id": "node"}`, goAfterNode, tool); err != nil {
		t.Fatalf("Setup: PushFeatures(features) = %v", err)
	}
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
		UpdateScripts:       *update,
		Setup: func(env *testscript.Env) error {
			env.Setenv("HOME", env.WorkDir)
			env.Setenv("REGISTRY", reg.Host)
			// Hand the scripts every value of the fixture set by its name
			for name, value := range fixture.Env() {
				env.Setenv(name, value)
			}
			bin := filepath.Join(env.WorkDir, ".bin")
			// Hand back a fake engine that stays unwritten
			if err := os.MkdirAll(bin, 0o750); err != nil {
				return err
			}
			// Hand back a fake engine that stays unwritten
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(fakeDocker), 0o755); err != nil { //nolint:gosec // the scripts run the fake engine
				return err
			}
			// Hand back a fake devcontainer CLI that stays unwritten
			if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte(fakeDevcontainer), 0o755); err != nil { //nolint:gosec // the scripts run the fake CLI
				return err
			}
			fakes := map[string]string{"aa-enabled": fakeAaEnabled, "limactl": fakeLimactl, "code": fakeCode, "wslpath": fakeWslpath, "cmd.exe": fakeCmd, "qemu-img": fakeQEMU, "qemu-system-x86_64": fakeQEMU, "qemu-system-aarch64": fakeQEMU}
			// Write every other fake the scripts call
			for name, script := range fakes {
				// Hand back a fake that stays unwritten
				if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil { //nolint:gosec // the scripts run the fakes
					return err
				}
			}
			// Name the folder testscript put o3s in, so a script can narrow PATH to the fakes and o3s
			for _, dir := range filepath.SplitList(env.Getenv("PATH")) {
				// Take the folder that holds o3s
				if _, err := os.Stat(filepath.Join(dir, "o3s")); err == nil {
					env.Setenv("O3S_DIR", dir)
					break
				}
			}
			env.Setenv("PATH", bin+string(os.PathListSeparator)+env.Getenv("PATH"))
			ptySetup(env)
			return nil
		},
		Cmds: ptyCmds,
	})
}
