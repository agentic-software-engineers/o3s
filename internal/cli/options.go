package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hansehart/o3s/internal/registry"
)

// given is one option the command line sets, for the feature it names or for whichever feature offers it.
type given struct {
	feature, name, value string
}

// parseOption reads an option given as name=value, or as id:name=value for the feature with that id.
func parseOption(arg string) (given, error) {
	left, value, ok := strings.Cut(arg, "=")
	// Turn away an option without a value or a name
	if !ok || left == "" {
		return given{}, fmt.Errorf("%w: invalid option %q, want name=value or id:name=value", ErrUsage, arg)
	}
	// Take the feature named before the last colon, since a store's address may hold one too
	if feature, name, ok := strings.CutLast(left, ":"); ok {
		return given{feature: feature, name: name, value: value}, nil
	}
	return given{name: left, value: value}, nil
}

// applyOptions sets every option the command line gives on the feature that offers it.
func applyOptions(setups []registry.Setup, args []string) error {
	// Set every option in the order given
	for _, arg := range args {
		g, err := parseOption(arg)
		// Hand back an option that fails to read
		if err != nil {
			return err
		}
		// Keep the folder o3s gives a feature to o3s
		if g.name == registry.StateDir {
			return fmt.Errorf("%w: %s is set by o3s", ErrUsage, registry.StateDir)
		}
		var offering []int
		// Find every feature that offers the option, or the one the option names
		for i, s := range setups {
			// Take a feature that offers the option and matches the name given, if any
			if offers(s, g.name) && (g.feature == "" || g.feature == s.Ref.ID || g.feature == s.Ref.Package()) {
				offering = append(offering, i)
			}
		}
		// Turn away an option no feature here offers
		if len(offering) == 0 {
			return fmt.Errorf("%w: no feature here offers the option %s", ErrUsage, g.name)
		}
		// Ask for the feature's id where several offer the option
		if len(offering) > 1 {
			var ids []string
			// Name every feature that offers it
			for _, i := range offering {
				ids = append(ids, setups[i].Ref.ID)
			}
			return fmt.Errorf("%w: %s both offer the option %s, name one as in --option %s:%s=%s", ErrUsage, joinAnd(ids), g.name, ids[len(ids)-1], g.name, g.value)
		}
		s := &setups[offering[0]]
		value, err := valueFor(option(*s, g.name), g.value)
		// Hand back a value the option takes none of
		if err != nil {
			return err
		}
		// Start the options of a feature that holds none yet
		if s.Options == nil {
			s.Options = map[string]any{}
		}
		s.Options[g.name] = value
	}
	return nil
}

// offers reports whether a feature offers an option.
func offers(s registry.Setup, name string) bool {
	return slices.ContainsFunc(s.Offered, func(o registry.Option) bool { return o.Name == name })
}

// option hands back the option of a feature by its name.
func option(s registry.Setup, name string) registry.Option {
	i := slices.IndexFunc(s.Offered, func(o registry.Option) bool { return o.Name == name })
	return s.Offered[i]
}

// valueFor turns a value into what an option takes, a boolean for a boolean and one of its choices where it has them.
func valueFor(o registry.Option, value string) (any, error) {
	// Read a boolean option's value as a boolean
	if o.Type == "boolean" {
		// Take the two words a boolean reads as
		switch value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, fmt.Errorf("%w: invalid value %q for %s, want true or false", ErrUsage, value, o.Name)
	}
	// Turn away a value outside the choices an option holds to
	if len(o.Enum) > 0 && !slices.Contains(o.Enum, value) {
		return nil, fmt.Errorf("%w: invalid value %q for %s, want %s", ErrUsage, value, o.Name, joinOr(o.Enum))
	}
	return value, nil
}

// joinOr joins choices the way a sentence offers them.
func joinOr(choices []string) string {
	// Join a single choice as it stands, and the last of several with or
	if len(choices) < 2 {
		return strings.Join(choices, "")
	}
	return strings.Join(choices[:len(choices)-1], ", ") + " or " + choices[len(choices)-1]
}
