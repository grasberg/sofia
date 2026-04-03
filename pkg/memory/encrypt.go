package memory

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const (
	encPrefix        = "enc::" // prefix to identify encrypted values
	pbkdf2Iter       = 100_000
	keyLen           = 32 // AES-256
	saltLen          = 16
	saltFile         = "db.salt"
	minPassphraseLen = 16
)

var (
	dbEncryptionKey []byte
	dbGCM           cipher.AEAD // cached GCM instance — reused across encrypt/decrypt calls
	encryptOnce     sync.Once
)

func initEncryptionKey() {
	encryptOnce.Do(func() {
		passphrase := os.Getenv("SOFIA_DB_KEY")
		if passphrase == "" {
			return
		}
		if len(passphrase) < minPassphraseLen {
			log.Printf(
				"[encrypt] WARNING: SOFIA_DB_KEY is too short (min %d chars), encryption disabled",
				minPassphraseLen,
			)
			return
		}
		salt, err := loadOrCreateSalt()
		if err != nil {
			log.Printf("[encrypt] WARNING: failed to load/create salt, encryption disabled: %v", err)
			return
		}
		dbEncryptionKey = pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iter, keyLen, sha256.New)

		block, err := aes.NewCipher(dbEncryptionKey)
		if err != nil {
			log.Printf("[encrypt] WARNING: failed to create AES cipher, encryption disabled: %v", err)
			dbEncryptionKey = nil
			return
		}
		dbGCM, err = cipher.NewGCM(block)
		if err != nil {
			log.Printf("[encrypt] WARNING: failed to create GCM, encryption disabled: %v", err)
			dbEncryptionKey = nil
			return
		}
	})
}

// EncryptionActive reports whether database encryption is enabled and initialized.
func EncryptionActive() bool {
	initEncryptionKey()
	return dbEncryptionKey != nil
}

// loadOrCreateSalt returns a per-installation random salt.
// On first run it generates 16 random bytes and writes them to ~/.sofia/db.salt.
// On subsequent runs it reads the existing salt file.
func loadOrCreateSalt() ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".sofia", saltFile)

	// Try reading existing salt.
	if data, err := os.ReadFile(path); err == nil && len(data) >= saltLen {
		return data[:saltLen], nil
	}

	// Generate new random salt.
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	// Ensure directory exists and write atomically.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("cannot create salt directory (read-only filesystem?): %w", err)
	}
	if err := os.WriteFile(path, salt, 0o600); err != nil {
		return nil, fmt.Errorf("cannot persist salt file: %w", err)
	}
	return salt, nil
}

// encryptValue encrypts plaintext using AES-256-GCM if SOFIA_DB_KEY is set.
// Returns the plaintext unchanged when no encryption key is configured.
func encryptValue(plaintext string) string {
	initEncryptionKey()
	if dbGCM == nil {
		return plaintext
	}

	nonce := make([]byte, dbGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Printf("[encrypt] WARNING: failed to generate nonce: %v", err)
		return plaintext
	}

	ciphertext := dbGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext)
}

// decryptValue decrypts a value previously encrypted by encryptValue.
// Unencrypted values (no "enc::" prefix) are returned as-is, making this
// backward compatible with existing data.
func decryptValue(value string) string {
	initEncryptionKey()
	if dbGCM == nil || !isEncrypted(value) {
		return value
	}

	data, err := base64.StdEncoding.DecodeString(value[len(encPrefix):])
	if err != nil {
		log.Printf("[encrypt] WARNING: failed to decode encrypted value: %v", err)
		return value
	}

	nonceSize := dbGCM.NonceSize()
	if len(data) < nonceSize {
		return value
	}

	plaintext, err := dbGCM.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		log.Printf("[encrypt] WARNING: decryption failed (wrong key or corrupted data)")
		return value
	}
	return string(plaintext)
}

// isEncrypted returns true when the value carries the encryption prefix.
func isEncrypted(value string) bool {
	return len(value) > len(encPrefix) && value[:len(encPrefix)] == encPrefix
}

// defaultEncryptor delegates to the package-level encrypt/decrypt functions
// backed by the SOFIA_DB_KEY environment variable. It satisfies the Encryptor
// interface so MemoryDB works out-of-the-box with zero configuration.
type defaultEncryptor struct{}

func (defaultEncryptor) Encrypt(plaintext string) string  { return encryptValue(plaintext) }
func (defaultEncryptor) Decrypt(ciphertext string) string { return decryptValue(ciphertext) }
func (defaultEncryptor) Active() bool                     { return EncryptionActive() }

// NopEncryptor is an Encryptor that performs no encryption.
// Useful in tests to avoid environment-dependent behavior.
type NopEncryptor struct{}

func (NopEncryptor) Encrypt(plaintext string) string  { return plaintext }
func (NopEncryptor) Decrypt(ciphertext string) string { return ciphertext }
func (NopEncryptor) Active() bool                     { return false }
