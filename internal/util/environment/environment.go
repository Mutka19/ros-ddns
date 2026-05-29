package environment

import (
	"fmt"
	"os"
	"strings"
)

func ReadSecret(name string) (string, error) {
	data, err := os.ReadFile("/run/secrets/" + name)
	if err != nil {
		return "", fmt.Errorf("reading secret %q: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}
