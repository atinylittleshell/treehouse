package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kunchenguid/treehouse/internal/deadline"
)

// Confirm asks the user a yes/no question on stderr and reads the answer from
// stdin.
//
// There is no bound on how long a person takes to answer, so the wait is not
// charged to the caller's deadline: Confirm grants a fresh budget on the way
// out. Without that, a prompt left sitting longer than the timeout would make
// the reset that follows the answer fail on an expired deadline.
func Confirm(message string, defaultYes bool) (bool, error) {
	defer deadline.Restart()

	hint := "Y/n"
	if !defaultYes {
		hint = "y/N"
	}

	fmt.Fprintf(os.Stderr, "%s [%s] ", message, hint)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultYes, err
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultYes, nil
	}

	return input == "y" || input == "yes", nil
}
