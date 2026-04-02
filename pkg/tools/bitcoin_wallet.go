package tools

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/scrypt"
)

// walletFile is the on-disk JSON format for the encrypted HD wallet.
type walletFile struct {
	Version          int    `json:"version"`
	Network          string `json:"network"`
	EncryptedSeed    string `json:"encrypted_seed"` // hex AES-256-GCM ciphertext
	Salt             string `json:"salt"`           // hex scrypt salt
	Nonce            string `json:"nonce"`          // hex GCM nonce
	NextReceiveIndex uint32 `json:"next_receive_index"`
	NextChangeIndex  uint32 `json:"next_change_index"`
}

// HDWallet is an in-memory BIP84 HD wallet.
type HDWallet struct {
	mu          sync.Mutex
	seed        []byte
	masterKey   *hdkeychain.ExtendedKey
	network     string
	netParams   *chaincfg.Params
	filePath    string
	passphrase  string
	nextReceive uint32
	nextChange  uint32
}

// netParamsFor returns chaincfg params for the given network name.
func netParamsFor(network string) *chaincfg.Params {
	switch network {
	case "testnet":
		return &chaincfg.TestNet3Params
	case "signet":
		return &chaincfg.SigNetParams
	default:
		return &chaincfg.MainNetParams
	}
}

// bip84CoinType returns the BIP44 coin type for the network.
func bip84CoinType(network string) uint32 {
	if network == "mainnet" {
		return 0
	}
	return 1 // testnet & signet
}

// createHDWallet generates a new BIP39 wallet and saves it encrypted.
func createHDWallet(passphrase, network, filePath string) (*HDWallet, string, error) {
	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return nil, "", fmt.Errorf("generate entropy: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, "", fmt.Errorf("generate mnemonic: %w", err)
	}

	w, err := walletFromMnemonic(mnemonic, passphrase, network, filePath)
	if err != nil {
		return nil, "", err
	}

	if err := w.save(); err != nil {
		return nil, "", fmt.Errorf("save wallet: %w", err)
	}

	return w, mnemonic, nil
}

// importHDWallet imports an existing BIP39 mnemonic and saves it encrypted.
func importHDWallet(mnemonic, passphrase, network, filePath string) (*HDWallet, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("invalid BIP39 mnemonic")
	}

	w, err := walletFromMnemonic(mnemonic, passphrase, network, filePath)
	if err != nil {
		return nil, err
	}

	if err := w.save(); err != nil {
		return nil, fmt.Errorf("save wallet: %w", err)
	}

	return w, nil
}

// walletFromMnemonic creates an HDWallet from a mnemonic.
func walletFromMnemonic(mnemonic, passphrase, network, filePath string) (*HDWallet, error) {
	seed := bip39.NewSeed(mnemonic, "")
	params := netParamsFor(network)

	masterKey, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		return nil, fmt.Errorf("derive master key: %w", err)
	}

	return &HDWallet{
		seed:       seed,
		masterKey:  masterKey,
		network:    network,
		netParams:  params,
		filePath:   filePath,
		passphrase: passphrase,
	}, nil
}

// loadHDWallet loads and decrypts a wallet from disk.
func loadHDWallet(passphrase, filePath string) (*HDWallet, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read wallet file: %w", err)
	}

	var wf walletFile
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse wallet file: %w", err)
	}

	if wf.Version != 1 {
		return nil, fmt.Errorf("unsupported wallet version %d", wf.Version)
	}

	salt, err := hex.DecodeString(wf.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}

	nonce, err := hex.DecodeString(wf.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}

	ciphertext, err := hex.DecodeString(wf.EncryptedSeed)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	seed, err := decryptAESGCM(key, nonce, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt wallet (wrong passphrase?): %w", err)
	}

	params := netParamsFor(wf.Network)
	masterKey, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		return nil, fmt.Errorf("derive master key: %w", err)
	}

	return &HDWallet{
		seed:        seed,
		masterKey:   masterKey,
		network:     wf.Network,
		netParams:   params,
		filePath:    filePath,
		passphrase:  passphrase,
		nextReceive: wf.NextReceiveIndex,
		nextChange:  wf.NextChangeIndex,
	}, nil
}

// save encrypts and writes the wallet to disk.
func (w *HDWallet) save() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}

	key, err := deriveKey(w.passphrase, salt)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	ciphertext, nonce, err := encryptAESGCM(key, w.seed)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	wf := walletFile{
		Version:          1,
		Network:          w.network,
		EncryptedSeed:    hex.EncodeToString(ciphertext),
		Salt:             hex.EncodeToString(salt),
		Nonce:            hex.EncodeToString(nonce),
		NextReceiveIndex: w.nextReceive,
		NextChangeIndex:  w.nextChange,
	}

	data, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	dir := filepath.Dir(w.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	return os.WriteFile(w.filePath, data, 0o600)
}

// deriveAddress derives a BIP84 address at the given chain (0=receive, 1=change) and index.
// Path: m/84'/<coin>'/<account>'/<chain>/<index>
func (w *HDWallet) deriveAddress(chain, index uint32) (string, *btcec.PrivateKey, error) {
	// m/84'
	purpose, err := w.masterKey.Derive(hdkeychain.HardenedKeyStart + 84)
	if err != nil {
		return "", nil, fmt.Errorf("derive purpose: %w", err)
	}
	// m/84'/<coin>'
	coinKey, err := purpose.Derive(hdkeychain.HardenedKeyStart + bip84CoinType(w.network))
	if err != nil {
		return "", nil, fmt.Errorf("derive coin: %w", err)
	}
	// m/84'/<coin>'/0'
	accountKey, err := coinKey.Derive(hdkeychain.HardenedKeyStart + 0)
	if err != nil {
		return "", nil, fmt.Errorf("derive account: %w", err)
	}
	// m/84'/<coin>'/0'/<chain>
	chainKey, err := accountKey.Derive(chain)
	if err != nil {
		return "", nil, fmt.Errorf("derive chain: %w", err)
	}
	// m/84'/<coin>'/0'/<chain>/<index>
	childKey, err := chainKey.Derive(index)
	if err != nil {
		return "", nil, fmt.Errorf("derive index: %w", err)
	}

	privKey, err := childKey.ECPrivKey()
	if err != nil {
		return "", nil, fmt.Errorf("extract privkey: %w", err)
	}

	pubKeyHash := btcutil.Hash160(privKey.PubKey().SerializeCompressed())
	addr, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, w.netParams)
	if err != nil {
		return "", nil, fmt.Errorf("create address: %w", err)
	}

	return addr.EncodeAddress(), privKey, nil
}

// nextReceiveAddress returns the next unused receive address and increments the index.
func (w *HDWallet) nextReceiveAddress() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	addr, _, err := w.deriveAddress(0, w.nextReceive)
	if err != nil {
		return "", err
	}

	w.nextReceive++
	return addr, nil
}

// nextChangeAddress returns the next unused change address and increments the index.
func (w *HDWallet) nextChangeAddress() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	addr, _, err := w.deriveAddress(1, w.nextChange)
	if err != nil {
		return "", err
	}

	w.nextChange++
	return addr, nil
}

// allAddresses returns all derived addresses (receive + change) up to the current indices.
func (w *HDWallet) allAddresses() ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var addrs []string
	for i := uint32(0); i <= w.nextReceive; i++ {
		addr, _, err := w.deriveAddress(0, i)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	for i := uint32(0); i < w.nextChange; i++ {
		addr, _, err := w.deriveAddress(1, i)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// addressKeyMap returns a map of address -> private key for all derived addresses.
func (w *HDWallet) addressKeyMap() (map[string]*btcec.PrivateKey, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	m := make(map[string]*btcec.PrivateKey)
	// Include one extra receive address beyond nextReceive for the current "tip"
	for i := uint32(0); i <= w.nextReceive; i++ {
		addr, key, err := w.deriveAddress(0, i)
		if err != nil {
			return nil, err
		}
		m[addr] = key
	}
	for i := uint32(0); i <= w.nextChange; i++ {
		addr, key, err := w.deriveAddress(1, i)
		if err != nil {
			return nil, err
		}
		m[addr] = key
	}
	return m, nil
}

// signTx signs all inputs of a transaction given the UTXO details.
func (w *HDWallet) signTx(
	tx *wire.MsgTx,
	utxos []walletUTXO,
	keyMap map[string]*btcec.PrivateKey,
) error {
	for i, u := range utxos {
		privKey, ok := keyMap[u.Address]
		if !ok {
			return fmt.Errorf("no key for address %s (input %d)", u.Address, i)
		}

		pubKeyHash := btcutil.Hash160(privKey.PubKey().SerializeCompressed())
		witnessScript, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).
			AddData(pubKeyHash).
			Script()
		if err != nil {
			return fmt.Errorf("build witness script: %w", err)
		}

		sigHashes := txscript.NewTxSigHashes(tx, txscript.NewCannedPrevOutputFetcher(
			witnessScript, u.Value,
		))

		witness, err := txscript.WitnessSignature(
			tx, sigHashes, i, u.Value,
			witnessScript, txscript.SigHashAll, privKey, true,
		)
		if err != nil {
			return fmt.Errorf("sign input %d: %w", i, err)
		}

		tx.TxIn[i].Witness = witness
	}

	return nil
}

// walletUTXO represents an unspent output owned by the wallet.
type walletUTXO struct {
	TxID    string
	Vout    uint32
	Address string
	Value   int64 // sats
}

// ── Encryption helpers ───────────────────────────────────────────────

// deriveKey derives a 32-byte AES key from a passphrase using scrypt.
// N=1<<17 (131072), r=8, p=1 provides strong resistance against brute-force attacks.
func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(passphrase), salt, 1<<17, 8, 1, 32)
}

// encryptAESGCM encrypts plaintext with AES-256-GCM.
func encryptAESGCM(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// decryptAESGCM decrypts AES-256-GCM ciphertext.
func decryptAESGCM(key, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return gcm.Open(nil, nonce, ciphertext, nil)
}

// passphraseHash returns a quick SHA-256 hash for display/verification (not for crypto).
func passphraseHash(passphrase string) string {
	h := sha256.Sum256([]byte(passphrase))
	return hex.EncodeToString(h[:4])
}
