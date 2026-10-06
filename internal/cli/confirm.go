package cli

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var errDeclined = errors.New("aborted")

// confirm asks a yes/no question on a terminal. Without a terminal it refuses
// unless --yes was given, so scripts never delete by accident.
func (o *globalOpts) confirm(cmd *cobra.Command, prompt string, yes bool) error {
	if yes {
		return nil
	}
	if !o.deps.isTerminal() {
		return usageErrorf("%s: refusing without --yes when stdin is not a terminal", strings.TrimSuffix(prompt, "?"))
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", prompt)
	answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return errDeclined
}
