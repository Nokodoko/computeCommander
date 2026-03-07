package holdout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// EncryptTests encrypts holdout tests to a file using the age CLI tool.
// The age tool must be installed and the recipient public key must exist.
func EncryptTests(tests []HoldoutTest, recipientKeyPath, outputPath string) error {
	data, err := json.Marshal(tests)
	if err != nil {
		return fmt.Errorf("marshal tests: %w", err)
	}

	// Use age CLI for encryption: age -R <recipient-key-file> -o <output>
	cmd := exec.Command("age", "-R", recipientKeyPath, "-o", outputPath)
	cmd.Stdin = bytes.NewReader(data)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age encrypt: %s: %w", stderr.String(), err)
	}

	return nil
}

// DecryptTests decrypts holdout tests from an encrypted file using the age identity.
func DecryptTests(encryptedPath, identityPath string) ([]HoldoutTest, error) {
	// Use age CLI for decryption: age -d -i <identity-file> <encrypted-file>
	cmd := exec.Command("age", "-d", "-i", identityPath, encryptedPath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("age decrypt: %s: %w", stderr.String(), err)
	}

	var tests []HoldoutTest
	if err := json.Unmarshal(stdout.Bytes(), &tests); err != nil {
		return nil, fmt.Errorf("unmarshal decrypted tests: %w", err)
	}

	return tests, nil
}

// GenerateKeyPair generates an age X25519 keypair using the age-keygen tool.
// Returns the identity (private) content and recipient (public) key string.
func GenerateKeyPair() (identityContent string, recipientKey string, err error) {
	cmd := exec.Command("age-keygen")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("age-keygen: %w", err)
	}

	identity := stdout.String()

	// Extract the public key from the comment line
	// age-keygen outputs: # public key: age1...
	for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
		if bytes.HasPrefix(line, []byte("# public key: ")) {
			recipientKey = string(bytes.TrimPrefix(line, []byte("# public key: ")))
			break
		}
	}

	return identity, recipientKey, nil
}

// WriteKeyFile writes an age identity or recipient key to a file.
func WriteKeyFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// ReadKeyFile reads an age key file.
func ReadKeyFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open key file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read key file: %w", err)
	}
	return string(data), nil
}
