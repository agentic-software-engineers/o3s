package stacktest

import (
	"fmt"
	"strconv"
	"strings"
)

// CapNetAdmin is the bit of CAP_NET_ADMIN in a capability set (include/uapi/linux/capability.h).
const CapNetAdmin = 12

// Holds reports whether the effective capabilities in a /proc/<pid>/status listing include the capability at bit.
func Holds(status string, bit int) (bool, error) {
	// Read the effective set from its line, a hex mask
	for line := range strings.Lines(status) {
		// Move on to the next line until the effective set's line
		if !strings.HasPrefix(line, "CapEff:") {
			continue
		}
		mask, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
		// Hand back a mask that fails to parse
		if err != nil {
			return false, fmt.Errorf("parse CapEff %q: %w", line, err)
		}
		return mask&(1<<bit) != 0, nil
	}
	return false, fmt.Errorf("no CapEff line in %q", status)
}
