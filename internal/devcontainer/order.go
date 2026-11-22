package devcontainer

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/tailscale/hujson"
)

// orderKey is the top-level member that fixes the order the features install in, as the Dev Container spec names it.
const orderKey = "overrideFeatureInstallOrder"

// ErrOrder marks an install order that puts a feature before one it installs after.
var ErrOrder = errors.New("invalid install order")

// InstallOrder hands back the order the config installs its features in, as store/id, and an empty list where it
// leaves the order open.
func (c *Config) InstallOrder() []string {
	// Look at every top-level member
	for _, m := range c.v.Value.(*hujson.Object).Members {
		// Read the order the config fixes
		if name(m) == orderKey {
			var order []string
			v := m.Value.Clone()
			v.Standardize()
			_ = json.Unmarshal(v.Pack(), &order)
			return order
		}
	}
	return nil
}

// SetInstallOrder fixes the order the features install in, and drops the member where the order is empty.
func (c *Config) SetInstallOrder(order []string) {
	root := c.v.Value.(*hujson.Object)
	i := slices.IndexFunc(root.Members, func(m hujson.ObjectMember) bool { return name(m) == orderKey })
	// Drop the member where the list of features to order is empty
	if len(order) == 0 {
		// Drop a member the config holds
		if i >= 0 {
			root.Members = slices.Delete(root.Members, i, i+1)
		}
		return
	}
	v := literal(order, c.unit(), c.unit())
	// Take the place of the order the config fixes already, keeping its comments
	if i >= 0 {
		v.BeforeExtra, v.AfterExtra = root.Members[i].Value.BeforeExtra, root.Members[i].Value.AfterExtra
		root.Members[i].Value = v
		return
	}
	appendMember(root, orderKey, v, c.unit(), "")
}

// Ordered works out the order features install in: the ones named first in that order, then the rest as they
// stand, every feature moved just as far as it must to follow the features it installs after. A first that puts a
// feature before one it installs after is turned away with both named.
func Ordered(features []string, after map[string][]string, first []string) ([]string, error) {
	base := slices.Clone(first)
	// Turn away a feature named first that is missing from the installed ones
	for _, f := range first {
		// Name the feature the home lacks
		if !slices.Contains(features, f) {
			return nil, fmt.Errorf("%w: %s is not installed", ErrOrder, f)
		}
	}
	// Follow the features named first with the rest, as they stand
	for _, f := range features {
		// Take a feature outside the ones named first
		if !slices.Contains(base, f) {
			base = append(base, f)
		}
	}
	// Turn away a first that puts a feature before one it installs after
	for i, f := range first {
		// Check every feature this one installs after
		for _, dep := range after[f] {
			// Name the feature that has to come first
			if j := slices.Index(base, dep); j > i {
				return nil, fmt.Errorf("%w: %s installs after %s, so %s comes first", ErrOrder, f, dep, dep)
			}
		}
	}
	var out []string
	left := slices.Clone(base)
	// Take, again and again, the first feature whose features to follow are all in place
	for len(left) > 0 {
		// Keep the order as it stands where features follow each other in a circle, taking the first one left
		i := max(slices.IndexFunc(left, func(f string) bool { return ready(f, after[f], left) }), 0)
		out = append(out, left[i])
		left = slices.Delete(left, i, i+1)
	}
	return out, nil
}

// ready reports whether every feature one installs after is in place, each one off the list of those still waiting.
func ready(f string, deps, left []string) bool {
	// Check every feature this one follows
	for _, dep := range deps {
		// Wait for a feature still left, other than itself
		if dep != f && slices.Contains(left, dep) {
			return false
		}
	}
	return true
}
