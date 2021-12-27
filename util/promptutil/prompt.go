package promptutil

import (
	"fmt"
	"strings"
)

// Prompt prompts user to pick a choice; if defaultChoice >= 0 and the only enter is pushed, return the corresponding
// value from the slice of choices.
func Prompt(msg string, choices []string, defaultChoice int, caseInsensitive bool) (string, error) {
	m := map[string]struct{}{}
	for _, s := range choices {
		if caseInsensitive {
			s = strings.ToLower(s)
		}
		m[s] = struct{}{}
	}
	for {
		fmt.Printf("%s", msg)

		var line string
		_, err := fmt.Scanln(&line)
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" && defaultChoice >= 0 {
			return choices[defaultChoice], nil
		}
		if caseInsensitive {
			line = strings.ToLower(line)
		}
		if _, ok := m[line]; ok {
			return line, nil
		}
		fmt.Printf("invalid choice\n")
	}
}
