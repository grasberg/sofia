package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BitcoinTool provides Bitcoin wallet and blockchain operations.
// Read-only queries use the Mempool.space public API (no auth needed).
// Wallet operations use a local BIP84 HD wallet (no daemon required).
type BitcoinTool struct {
	network    string // "mainnet", "testnet", "signet"
	mempoolAPI string
	walletPath string // path to encrypted wallet file
	passphrase string // wallet encryption passphrase
	client     *http.Client

	// Confirmation tokens for send operations
	mu            sync.Mutex
	pendingTokens map[string]pendingSend
}

// pendingSend holds the details of a send operation awaiting confirmation.
type pendingSend struct {
	ToAddr     string
	AmountSats int64
	AmountBTC  string
	FeeRate    int64
	Expires    time.Time
}

// NewBitcoinTool creates a new Bitcoin tool.
func NewBitcoinTool(network, walletPath, passphrase string) *BitcoinTool {
	if network == "" {
		network = "mainnet"
	}

	// Default wallet path to ~/.sofia/bitcoin_wallet.json so files never
	// end up next to the binary.
	if walletPath == "" {
		home, _ := os.UserHomeDir()
		walletPath = filepath.Join(home, ".sofia", "bitcoin_wallet.json")
	}

	mempoolAPI := "https://mempool.space/api"
	switch network {
	case "testnet":
		mempoolAPI = "https://mempool.space/testnet/api"
	case "signet":
		mempoolAPI = "https://mempool.space/signet/api"
	}

	return &BitcoinTool{
		network:       network,
		mempoolAPI:    mempoolAPI,
		walletPath:    walletPath,
		passphrase:    passphrase,
		client:        &http.Client{Timeout: 30 * time.Second},
		pendingTokens: make(map[string]pendingSend),
	}
}

func (t *BitcoinTool) Name() string {
	return "bitcoin"
}

func (t *BitcoinTool) Description() string {
	return "Bitcoin wallet and blockchain tool. NO bitcoin-cli or local node required — uses mempool.space API. " +
		"ALWAYS use this tool for anything Bitcoin-related. NEVER use exec/shell/curl. " +
		"Public (no wallet): price, fee_estimate, balance (any address), transactions, utxos, tx_info. " +
		"Wallet: create_wallet, import_wallet, new_address, wallet_balance, wallet_addresses. " +
		"Send (TWO-STEP): 1) call send with to_address + amount_btc → get confirmation_token. " +
		"2) Show details to user, get approval, then call send with confirmation_token to broadcast. " +
		"Examples: {action:'price'}, {action:'wallet_balance'}, {action:'balance',address:'bc1q...'}, " +
		"{action:'send',to_address:'bc1q...',amount_btc:'0.001'}."
}

func (t *BitcoinTool) Parameters() map[string]any {
	var schema map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": [
					"balance", "transactions", "utxos", "tx_info", "fee_estimate", "price",
					"create_wallet", "import_wallet", "new_address", "wallet_balance",
					"wallet_addresses", "send"
				],
				"description": "Action to perform. Use 'wallet_balance' to check your HD wallet balance (no bitcoin-cli needed). Use 'balance' with an 'address' to check any address."
			},
			"address": {
				"type": "string",
				"description": "Bitcoin address (for balance, transactions, utxos)"
			},
			"txid": {
				"type": "string",
				"description": "Transaction ID (for tx_info)"
			},
			"to_address": {
				"type": "string",
				"description": "Destination address (for send)"
			},
			"amount_btc": {
				"type": "string",
				"description": "Amount in BTC (for send, e.g. '0.001')"
			},
			"fee_rate": {
				"type": "number",
				"description": "Fee rate in sat/vB (for send). If omitted, uses recommended fee."
			},
			"mnemonic": {
				"type": "string",
				"description": "BIP39 mnemonic phrase (for import_wallet, 12 or 24 words)"
			},
			"label": {
				"type": "string",
				"description": "Label for new address"
			},
			"confirmation_token": {
				"type": "string",
				"description": "Confirmation token for send action. First call to send returns a token with transaction details; pass this token back to confirm and broadcast."
			}
		},
		"required": ["action"]
	}`), &schema)
	return schema
}

func (t *BitcoinTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	// ── Public API — no wallet needed ──
	case "balance":
		return t.addressBalance(ctx, args)
	case "transactions":
		return t.addressTransactions(ctx, args)
	case "utxos":
		return t.addressUTXOs(ctx, args)
	case "tx_info":
		return t.txInfo(ctx, args)
	case "fee_estimate":
		return t.feeEstimate(ctx)
	case "price":
		return t.btcPrice(ctx)
	// ── Wallet operations — local HD wallet ──
	case "create_wallet":
		return t.createWallet()
	case "import_wallet":
		return t.importWallet(args)
	case "new_address":
		return t.newAddress()
	case "wallet_balance":
		return t.walletBalance(ctx)
	case "wallet_addresses":
		return t.walletAddresses()
	case "send":
		return t.sendBitcoin(ctx, args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action %q", action))
	}
}
