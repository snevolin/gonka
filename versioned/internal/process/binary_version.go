package process

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	printBinaryVersionFlag   = "--print-binary-version"
	printProtocolVersionFlag = "--print-protocol-version"
)

// readEmbeddedVersion runs binPath with flag and returns trimmed stdout.
func readEmbeddedVersion(binPath, flag string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, flag)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", binPath, flag, err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", fmt.Errorf("%s %s: empty output", binPath, flag)
	}
	return v, nil
}

func readBinaryLogVersion(binPath string) (string, error) {
	return readEmbeddedVersion(binPath, printBinaryVersionFlag)
}

func readProtocolVersion(binPath string) (string, error) {
	return readEmbeddedVersion(binPath, printProtocolVersionFlag)
}
