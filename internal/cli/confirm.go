package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// confirm asks whether to take an action, named by its verb, takes --yes as the answer, and turns away a missing
// terminal, naming the flag that takes the action there.
func confirm(in io.Reader, out io.Writer, terminal, yes bool, action string) (bool, error) {
	// Take the action without asking where the user said so
	if yes {
		return true, nil
	}
	// Turn away a run where nobody can answer
	if !terminal {
		return false, fmt.Errorf("%w: pass --yes to %s without a terminal", ErrUsage, action)
	}
	fmt.Fprintf(out, "%s? [y/N] ", strings.ToUpper(action[:1])+action[1:])
	line, err := bufio.NewReader(in).ReadString('\n')
	// Read an answer that ends without a line break as it stands, and a failed read as a no
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
