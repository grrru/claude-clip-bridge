package bridge

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// ReadTokenFile reads a hex-encoded 32-byte token from the given file path.
func ReadTokenFile(path string) ([TokenSize]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [TokenSize]byte{}, err
	}

	hexStr := strings.TrimSpace(string(data))
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return [TokenSize]byte{}, fmt.Errorf("token file %s: invalid hex: %w", path, err)
	}

	if len(decoded) != TokenSize {
		return [TokenSize]byte{}, fmt.Errorf("token file %s: expected %d bytes, got %d", path, TokenSize, len(decoded))
	}

	var token [TokenSize]byte
	copy(token[:], decoded)
	return token, nil
}
