// Command vsockhost is the host side of the cage VM that holds no network device: an allow-list proxy the guest reaches over vsock.
//
// proxy-unix PATH ALLOW LOG: serve the proxy on the Unix socket PATH (macOS, behind vfkit), names from ALLOW, refusals to LOG.
// run DIR: on Windows, boot DIR/disk.vhdx with DIR/seed.iso under HCS and serve the proxy on Hyper-V socket port 3128.
// stdio DIR PORT: on Windows, carry stdin and stdout to the guest's vsock PORT, for an SSH ProxyCommand.
package main

import (
	"log"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("usage: vsockhost proxy-unix PATH ALLOW LOG | run DIR | stdio DIR PORT")
	}
	// Pick the command, leaving the Windows ones to the platform
	if os.Args[1] == "proxy-unix" && len(os.Args) == 5 {
		proxyUnix(os.Args[2], os.Args[3], os.Args[4])
		return
	}
	platform(os.Args[1:])
}
