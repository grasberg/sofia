package tools

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
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
	return "Bitcoin wallet and blockchain tool. Query balances, transactions, UTXOs, fees, and price " +
		"without any setup. Create or import a BIP84 HD wallet locally (no daemon required) to " +
		"generate addresses, check wallet balance, and send bitcoin. " +
		"Actions: balance, transactions, utxos, tx_info, fee_estimate, price, " +
		"create_wallet, import_wallet, new_address, wallet_balance, wallet_addresses, send."
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
				"description": "Action to perform. Public (no wallet): balance, transactions, utxos, tx_info, fee_estimate, price. Wallet: create_wallet, import_wallet, new_address, wallet_balance, wallet_addresses, send."
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

// ── Mempool.space API helpers ────────────────────────────────────────

func (t *BitcoinTool) mempoolGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.mempoolAPI+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 300))
	}

	return body, nil
}

func (t *BitcoinTool) mempoolPost(ctx context.Context, path string, rawBody []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.mempoolAPI+path, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 500))
	}

	return body, nil
}

// ── Public blockchain queries ────────────────────────────────────────

func (t *BitcoinTool) addressBalance(ctx context.Context, args map[string]any) *ToolResult {
	addr := getStr(args, "address")
	if addr == "" {
		return ErrorResult("address is required for balance")
	}

	body, err := t.mempoolGet(ctx, "/address/"+addr)
	if err != nil {
		return RetryableError(fmt.Sprintf("balance query failed: %v", err), "Check address or try again")
	}

	var data struct {
		Address string `json:"address"`
		Chain   struct {
			FundedSats int64 `json:"funded_txo_sum"`
			SpentSats  int64 `json:"spent_txo_sum"`
			TxCount    int   `json:"tx_count"`
		} `json:"chain_stats"`
		Mempool struct {
			FundedSats int64 `json:"funded_txo_sum"`
			SpentSats  int64 `json:"spent_txo_sum"`
			TxCount    int   `json:"tx_count"`
		} `json:"mempool_stats"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return ErrorResult(fmt.Sprintf("parse balance: %v", err))
	}

	confirmed := data.Chain.FundedSats - data.Chain.SpentSats
	unconfirmed := data.Mempool.FundedSats - data.Mempool.SpentSats

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Address:** `%s`\n", addr))
	sb.WriteString(fmt.Sprintf("**Network:** %s\n\n", t.network))
	sb.WriteString(fmt.Sprintf("**Confirmed balance:** %s BTC (%d sats)\n", satsToBTC(confirmed), confirmed))
	if unconfirmed != 0 {
		sb.WriteString(fmt.Sprintf("**Unconfirmed:** %s BTC (%d sats)\n", satsToBTC(unconfirmed), unconfirmed))
	}
	sb.WriteString(fmt.Sprintf("**Total:** %s BTC\n", satsToBTC(confirmed+unconfirmed)))
	sb.WriteString(fmt.Sprintf("**Transactions:** %d confirmed", data.Chain.TxCount))
	if data.Mempool.TxCount > 0 {
		sb.WriteString(fmt.Sprintf(", %d unconfirmed", data.Mempool.TxCount))
	}
	sb.WriteString("\n")

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) addressTransactions(ctx context.Context, args map[string]any) *ToolResult {
	addr := getStr(args, "address")
	if addr == "" {
		return ErrorResult("address is required for transactions")
	}

	body, err := t.mempoolGet(ctx, "/address/"+addr+"/txs")
	if err != nil {
		return RetryableError(fmt.Sprintf("tx query failed: %v", err), "Check address or try again")
	}

	var txs []struct {
		TXID   string `json:"txid"`
		Status struct {
			Confirmed   bool  `json:"confirmed"`
			BlockHeight int64 `json:"block_height"`
			BlockTime   int64 `json:"block_time"`
		} `json:"status"`
		Fee int64 `json:"fee"`
		VIn []struct {
			Prevout struct {
				ScriptPubKeyAddr string `json:"scriptpubkey_address"`
				Value            int64  `json:"value"`
			} `json:"prevout"`
		} `json:"vin"`
		VOut []struct {
			ScriptPubKeyAddr string `json:"scriptpubkey_address"`
			Value            int64  `json:"value"`
		} `json:"vout"`
	}
	if err := json.Unmarshal(body, &txs); err != nil {
		return ErrorResult(fmt.Sprintf("parse transactions: %v", err))
	}

	if len(txs) == 0 {
		return NewToolResult(fmt.Sprintf("No transactions found for `%s`.", addr))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Transactions for `%s`** (latest %d):\n\n", addr, min(len(txs), 25)))

	for i, tx := range txs {
		if i >= 25 {
			sb.WriteString(fmt.Sprintf("\n*... and %d more transactions*\n", len(txs)-25))
			break
		}

		var received, sent int64
		for _, vin := range tx.VIn {
			if vin.Prevout.ScriptPubKeyAddr == addr {
				sent += vin.Prevout.Value
			}
		}
		for _, vout := range tx.VOut {
			if vout.ScriptPubKeyAddr == addr {
				received += vout.Value
			}
		}
		net := received - sent

		status := "Unconfirmed"
		timeStr := ""
		if tx.Status.Confirmed {
			status = fmt.Sprintf("Block %d", tx.Status.BlockHeight)
			if tx.Status.BlockTime > 0 {
				timeStr = time.Unix(tx.Status.BlockTime, 0).Format("2006-01-02 15:04")
			}
		}

		direction := "Received"
		if net < 0 {
			direction = "Sent"
		}

		sb.WriteString(fmt.Sprintf("**%d.** %s %s BTC\n", i+1, direction, satsToBTC(abs64(net))))
		sb.WriteString(fmt.Sprintf("   TXID: `%s`\n", tx.TXID[:16]+"..."))
		sb.WriteString(fmt.Sprintf("   %s", status))
		if timeStr != "" {
			sb.WriteString(fmt.Sprintf(" | %s", timeStr))
		}
		sb.WriteString(fmt.Sprintf(" | Fee: %d sats\n\n", tx.Fee))
	}

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) addressUTXOs(ctx context.Context, args map[string]any) *ToolResult {
	addr := getStr(args, "address")
	if addr == "" {
		return ErrorResult("address is required for utxos")
	}

	body, err := t.mempoolGet(ctx, "/address/"+addr+"/utxo")
	if err != nil {
		return RetryableError(fmt.Sprintf("UTXO query failed: %v", err), "Check address or try again")
	}

	var utxos []struct {
		TXID   string `json:"txid"`
		Vout   int    `json:"vout"`
		Status struct {
			Confirmed   bool  `json:"confirmed"`
			BlockHeight int64 `json:"block_height"`
		} `json:"status"`
		Value int64 `json:"value"`
	}
	if err := json.Unmarshal(body, &utxos); err != nil {
		return ErrorResult(fmt.Sprintf("parse UTXOs: %v", err))
	}

	if len(utxos) == 0 {
		return NewToolResult(fmt.Sprintf("No UTXOs found for `%s`.", addr))
	}

	var total int64
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**UTXOs for `%s`** (%d):\n\n", addr, len(utxos)))
	sb.WriteString("| # | Value (BTC) | Value (sats) | Confirmed | TXID:vout |\n")
	sb.WriteString("|---|-------------|--------------|-----------|------------|\n")

	for i, u := range utxos {
		conf := "no"
		if u.Status.Confirmed {
			conf = "yes"
		}
		total += u.Value
		sb.WriteString(fmt.Sprintf("| %d | %s | %d | %s | `%s:%d` |\n",
			i+1, satsToBTC(u.Value), u.Value, conf, u.TXID[:12]+"...", u.Vout))
	}
	sb.WriteString(fmt.Sprintf("\n**Total:** %s BTC (%d sats)\n", satsToBTC(total), total))

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) txInfo(ctx context.Context, args map[string]any) *ToolResult {
	txid := getStr(args, "txid")
	if txid == "" {
		return ErrorResult("txid is required for tx_info")
	}

	body, err := t.mempoolGet(ctx, "/tx/"+txid)
	if err != nil {
		return RetryableError(fmt.Sprintf("tx query failed: %v", err), "Check TXID or try again")
	}

	var tx struct {
		TXID    string `json:"txid"`
		Version int    `json:"version"`
		Size    int    `json:"size"`
		Weight  int    `json:"weight"`
		Fee     int64  `json:"fee"`
		Status  struct {
			Confirmed   bool   `json:"confirmed"`
			BlockHeight int64  `json:"block_height"`
			BlockHash   string `json:"block_hash"`
			BlockTime   int64  `json:"block_time"`
		} `json:"status"`
		VIn []struct {
			TXID    string `json:"txid"`
			Vout    int    `json:"vout"`
			Prevout struct {
				ScriptPubKeyAddr string `json:"scriptpubkey_address"`
				Value            int64  `json:"value"`
			} `json:"prevout"`
		} `json:"vin"`
		VOut []struct {
			ScriptPubKeyAddr string `json:"scriptpubkey_address"`
			Value            int64  `json:"value"`
		} `json:"vout"`
	}
	if err := json.Unmarshal(body, &tx); err != nil {
		return ErrorResult(fmt.Sprintf("parse tx: %v", err))
	}

	var totalIn, totalOut int64
	for _, vin := range tx.VIn {
		totalIn += vin.Prevout.Value
	}
	for _, vout := range tx.VOut {
		totalOut += vout.Value
	}

	vsize := tx.Weight / 4
	feeRate := float64(0)
	if vsize > 0 {
		feeRate = float64(tx.Fee) / float64(vsize)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Transaction:** `%s`\n\n", tx.TXID))

	if tx.Status.Confirmed {
		timeStr := time.Unix(tx.Status.BlockTime, 0).Format("2006-01-02 15:04 UTC")
		sb.WriteString(fmt.Sprintf("**Status:** Confirmed at block %d\n", tx.Status.BlockHeight))
		sb.WriteString(fmt.Sprintf("**Time:** %s\n", timeStr))
	} else {
		sb.WriteString("**Status:** Unconfirmed (in mempool)\n")
	}

	sb.WriteString(fmt.Sprintf("**Size:** %d vB | **Fee:** %d sats (%.1f sat/vB)\n\n", vsize, tx.Fee, feeRate))

	sb.WriteString(fmt.Sprintf("**Inputs (%d):** %s BTC\n", len(tx.VIn), satsToBTC(totalIn)))
	for i, vin := range tx.VIn {
		sb.WriteString(
			fmt.Sprintf("  %d. `%s` — %s BTC\n", i+1, vin.Prevout.ScriptPubKeyAddr, satsToBTC(vin.Prevout.Value)),
		)
	}

	sb.WriteString(fmt.Sprintf("\n**Outputs (%d):** %s BTC\n", len(tx.VOut), satsToBTC(totalOut)))
	for i, vout := range tx.VOut {
		sb.WriteString(fmt.Sprintf("  %d. `%s` — %s BTC\n", i+1, vout.ScriptPubKeyAddr, satsToBTC(vout.Value)))
	}

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) feeEstimate(ctx context.Context) *ToolResult {
	body, err := t.mempoolGet(ctx, "/v1/fees/recommended")
	if err != nil {
		return RetryableError(fmt.Sprintf("fee estimate failed: %v", err), "Try again")
	}

	var fees struct {
		FastestFee  int `json:"fastestFee"`
		HalfHourFee int `json:"halfHourFee"`
		HourFee     int `json:"hourFee"`
		EconomyFee  int `json:"economyFee"`
		MinimumFee  int `json:"minimumFee"`
	}
	if err := json.Unmarshal(body, &fees); err != nil {
		return ErrorResult(fmt.Sprintf("parse fees: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("**Current Bitcoin Fee Estimates** (sat/vB):\n\n")
	sb.WriteString("| Priority | Fee Rate | ~Cost (140 vB tx) |\n")
	sb.WriteString("|----------|----------|-------------------|\n")
	sb.WriteString(fmt.Sprintf("| Fastest (~10 min) | %d sat/vB | %d sats |\n", fees.FastestFee, fees.FastestFee*140))
	sb.WriteString(fmt.Sprintf("| Half hour | %d sat/vB | %d sats |\n", fees.HalfHourFee, fees.HalfHourFee*140))
	sb.WriteString(fmt.Sprintf("| Hour | %d sat/vB | %d sats |\n", fees.HourFee, fees.HourFee*140))
	sb.WriteString(fmt.Sprintf("| Economy | %d sat/vB | %d sats |\n", fees.EconomyFee, fees.EconomyFee*140))
	sb.WriteString(fmt.Sprintf("| Minimum | %d sat/vB | %d sats |\n", fees.MinimumFee, fees.MinimumFee*140))
	sb.WriteString(fmt.Sprintf("\n**Network:** %s\n", t.network))

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) btcPrice(ctx context.Context) *ToolResult {
	body, err := t.mempoolGet(ctx, "/v1/prices")
	if err != nil {
		return RetryableError(fmt.Sprintf("price query failed: %v", err), "Try again")
	}

	var prices struct {
		USD float64 `json:"USD"`
		EUR float64 `json:"EUR"`
		GBP float64 `json:"GBP"`
		CAD float64 `json:"CAD"`
		CHF float64 `json:"CHF"`
		AUD float64 `json:"AUD"`
		JPY float64 `json:"JPY"`
	}
	if err := json.Unmarshal(body, &prices); err != nil {
		return ErrorResult(fmt.Sprintf("parse prices: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("**Bitcoin Price:**\n\n")
	sb.WriteString("| Currency | Price |\n")
	sb.WriteString("|----------|-------|\n")
	sb.WriteString(fmt.Sprintf("| USD | $%.2f |\n", prices.USD))
	sb.WriteString(fmt.Sprintf("| EUR | %.2f |\n", prices.EUR))
	sb.WriteString(fmt.Sprintf("| GBP | %.2f |\n", prices.GBP))
	sb.WriteString(fmt.Sprintf("| CHF | CHF %.2f |\n", prices.CHF))
	sb.WriteString(fmt.Sprintf("| CAD | C$%.2f |\n", prices.CAD))
	sb.WriteString(fmt.Sprintf("| AUD | A$%.2f |\n", prices.AUD))
	sb.WriteString(fmt.Sprintf("| JPY | %.0f |\n", prices.JPY))

	if prices.USD > 0 {
		satsPerDollar := 100_000_000 / prices.USD
		sb.WriteString(fmt.Sprintf("\n**1 USD = %.0f sats**\n", satsPerDollar))
	}

	return NewToolResult(sb.String())
}

// ── Wallet operations ────────────────────────────────────────────────

func (t *BitcoinTool) loadWallet() (*HDWallet, error) {
	if t.walletPath == "" {
		return nil, fmt.Errorf(
			"no wallet configured — use create_wallet first, or set wallet path in Settings > Integrations > Bitcoin",
		)
	}
	if t.passphrase == "" {
		return nil, fmt.Errorf("wallet passphrase not set — configure it in Settings > Integrations > Bitcoin")
	}
	return loadHDWallet(t.passphrase, t.walletPath)
}

func (t *BitcoinTool) createWallet() *ToolResult {
	if t.walletPath == "" {
		return ErrorResult("wallet path not configured — set it in Settings > Integrations > Bitcoin")
	}
	if t.passphrase == "" {
		return ErrorResult("wallet passphrase not set — configure it in Settings > Integrations > Bitcoin")
	}

	w, mnemonic, err := createHDWallet(t.passphrase, t.network, t.walletPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("create wallet failed: %v", err))
	}

	// Generate the first receive address
	addr, err := w.nextReceiveAddress()
	if err != nil {
		return ErrorResult(fmt.Sprintf("derive first address: %v", err))
	}

	if err := w.save(); err != nil {
		return ErrorResult(fmt.Sprintf("save wallet: %v", err))
	}

	// Write mnemonic to a secure file instead of returning it in tool output.
	recoveryPath := filepath.Join(filepath.Dir(t.walletPath), "wallet_recovery.txt")
	recoveryContent := fmt.Sprintf(
		"BITCOIN WALLET RECOVERY PHRASE\n"+
			"==============================\n"+
			"Network: %s\n"+
			"Created: %s\n\n"+
			"%s\n\n"+
			"IMPORTANT: This is the ONLY way to recover your wallet.\n"+
			"Store this file in a safe place and NEVER share it with anyone.\n"+
			"Delete this file after you have securely backed up the phrase.\n",
		t.network,
		time.Now().Format("2006-01-02 15:04:05"),
		mnemonic,
	)
	if mkErr := os.MkdirAll(filepath.Dir(recoveryPath), 0o700); mkErr != nil {
		return ErrorResult(fmt.Sprintf("create recovery dir: %v", mkErr))
	}
	if writeErr := os.WriteFile(recoveryPath, []byte(recoveryContent), 0o600); writeErr != nil {
		return ErrorResult(fmt.Sprintf("write recovery file: %v", writeErr))
	}

	var sb strings.Builder
	sb.WriteString("**Wallet created successfully!**\n\n")
	sb.WriteString(fmt.Sprintf("**Recovery phrase saved to:** `%s`\n", recoveryPath))
	sb.WriteString("**IMPORTANT:** Open that file to back up your recovery phrase, then delete it.\n")
	sb.WriteString("The recovery phrase is the ONLY way to recover your wallet. Never share it.\n\n")
	sb.WriteString(fmt.Sprintf("**First receive address:** `%s`\n", addr))
	sb.WriteString(fmt.Sprintf("**Network:** %s\n", t.network))
	sb.WriteString(fmt.Sprintf("**Wallet file:** `%s`\n", t.walletPath))

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) importWallet(args map[string]any) *ToolResult {
	mnemonic := getStr(args, "mnemonic")
	if mnemonic == "" {
		return ErrorResult("mnemonic is required for import_wallet (12 or 24 words)")
	}
	if t.walletPath == "" {
		return ErrorResult("wallet path not configured — set it in Settings > Integrations > Bitcoin")
	}
	if t.passphrase == "" {
		return ErrorResult("wallet passphrase not set — configure it in Settings > Integrations > Bitcoin")
	}

	w, err := importHDWallet(mnemonic, t.passphrase, t.network, t.walletPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("import wallet failed: %v", err))
	}

	addr, err := w.nextReceiveAddress()
	if err != nil {
		return ErrorResult(fmt.Sprintf("derive address: %v", err))
	}

	if err := w.save(); err != nil {
		return ErrorResult(fmt.Sprintf("save wallet: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("**Wallet imported successfully!**\n\n")
	sb.WriteString(fmt.Sprintf("**First receive address:** `%s`\n", addr))
	sb.WriteString(fmt.Sprintf("**Network:** %s\n", t.network))
	sb.WriteString(fmt.Sprintf("**Wallet file:** `%s`\n", t.walletPath))

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) newAddress() *ToolResult {
	w, err := t.loadWallet()
	if err != nil {
		return ErrorResult(err.Error())
	}

	addr, err := w.nextReceiveAddress()
	if err != nil {
		return ErrorResult(fmt.Sprintf("derive address failed: %v", err))
	}

	if err := w.save(); err != nil {
		return ErrorResult(fmt.Sprintf("save wallet: %v", err))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**New receive address:** `%s`\n", addr))
	sb.WriteString(fmt.Sprintf("**Network:** %s\n", t.network))
	sb.WriteString(fmt.Sprintf("**Address index:** %d\n", w.nextReceive-1))

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) walletAddresses() *ToolResult {
	w, err := t.loadWallet()
	if err != nil {
		return ErrorResult(err.Error())
	}

	addrs, err := w.allAddresses()
	if err != nil {
		return ErrorResult(fmt.Sprintf("list addresses: %v", err))
	}

	if len(addrs) == 0 {
		return NewToolResult("No addresses generated yet. Use `new_address` to create one.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Wallet addresses** (%d):\n\n", len(addrs)))

	receiveCount := int(w.nextReceive) + 1
	for i, addr := range addrs {
		if i < receiveCount {
			sb.WriteString(fmt.Sprintf("  %d. `%s` (receive #%d)\n", i+1, addr, i))
		} else {
			sb.WriteString(fmt.Sprintf("  %d. `%s` (change #%d)\n", i+1, addr, i-receiveCount))
		}
	}

	sb.WriteString(fmt.Sprintf("\n**Network:** %s\n", t.network))

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) walletBalance(ctx context.Context) *ToolResult {
	w, err := t.loadWallet()
	if err != nil {
		return ErrorResult(err.Error())
	}

	addrs, err := w.allAddresses()
	if err != nil {
		return ErrorResult(fmt.Sprintf("list addresses: %v", err))
	}

	if len(addrs) == 0 {
		return NewToolResult("No addresses in wallet. Use `new_address` first.")
	}

	var totalConfirmed, totalUnconfirmed int64

	for _, addr := range addrs {
		body, err := t.mempoolGet(ctx, "/address/"+addr)
		if err != nil {
			continue // skip unreachable addresses
		}

		var data struct {
			Chain struct {
				FundedSats int64 `json:"funded_txo_sum"`
				SpentSats  int64 `json:"spent_txo_sum"`
			} `json:"chain_stats"`
			Mempool struct {
				FundedSats int64 `json:"funded_txo_sum"`
				SpentSats  int64 `json:"spent_txo_sum"`
			} `json:"mempool_stats"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			continue
		}

		totalConfirmed += data.Chain.FundedSats - data.Chain.SpentSats
		totalUnconfirmed += data.Mempool.FundedSats - data.Mempool.SpentSats
	}

	var sb strings.Builder
	sb.WriteString("**Wallet Balance:**\n\n")
	sb.WriteString(fmt.Sprintf("**Confirmed:** %s BTC (%d sats)\n", satsToBTC(totalConfirmed), totalConfirmed))
	if totalUnconfirmed != 0 {
		sb.WriteString(
			fmt.Sprintf("**Unconfirmed:** %s BTC (%d sats)\n", satsToBTC(totalUnconfirmed), totalUnconfirmed),
		)
	}
	sb.WriteString(fmt.Sprintf("**Total:** %s BTC\n", satsToBTC(totalConfirmed+totalUnconfirmed)))
	sb.WriteString(fmt.Sprintf("**Addresses scanned:** %d\n", len(addrs)))
	sb.WriteString(fmt.Sprintf("**Network:** %s\n", t.network))

	return NewToolResult(sb.String())
}

func (t *BitcoinTool) sendBitcoin(ctx context.Context, args map[string]any) *ToolResult {
	// Clean up expired tokens
	t.mu.Lock()
	now := time.Now()
	for tok, ps := range t.pendingTokens {
		if now.After(ps.Expires) {
			delete(t.pendingTokens, tok)
		}
	}
	t.mu.Unlock()

	// Check if this is a confirmation of a previous send request.
	confirmToken := getStr(args, "confirmation_token")
	if confirmToken != "" {
		return t.confirmSend(ctx, confirmToken)
	}

	toAddr := getStr(args, "to_address")
	amountStr := getStr(args, "amount_btc")

	if toAddr == "" {
		return ErrorResult("to_address is required for send")
	}
	if amountStr == "" {
		return ErrorResult("amount_btc is required for send")
	}

	var amountBTC float64
	if _, err := fmt.Sscanf(amountStr, "%f", &amountBTC); err != nil || amountBTC <= 0 {
		return ErrorResult("amount_btc must be a positive number (e.g. '0.001')")
	}
	amountSats := int64(amountBTC * 1e8)

	// Get fee rate
	feeRateSatVB := int64(2) // default
	if rate, ok := args["fee_rate"].(float64); ok && rate > 0 {
		feeRateSatVB = int64(rate)
	} else {
		body, err := t.mempoolGet(ctx, "/v1/fees/recommended")
		if err == nil {
			var fees struct {
				HalfHourFee int `json:"halfHourFee"`
			}
			if json.Unmarshal(body, &fees) == nil && fees.HalfHourFee > 0 {
				feeRateSatVB = int64(fees.HalfHourFee)
			}
		}
	}

	// Generate confirmation token and return transaction details for review.
	token := fmt.Sprintf("btc_send_%d", time.Now().UnixNano()/1000)
	t.mu.Lock()
	t.pendingTokens[token] = pendingSend{
		ToAddr:     toAddr,
		AmountSats: amountSats,
		AmountBTC:  amountStr,
		FeeRate:    feeRateSatVB,
		Expires:    time.Now().Add(5 * time.Minute),
	}
	t.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("**Bitcoin send requires confirmation.**\n\n")
	sb.WriteString(fmt.Sprintf("**To:** `%s`\n", toAddr))
	sb.WriteString(fmt.Sprintf("**Amount:** %s BTC (%d sats)\n", amountStr, amountSats))
	sb.WriteString(fmt.Sprintf("**Fee rate:** ~%d sat/vB\n", feeRateSatVB))
	sb.WriteString(fmt.Sprintf("**Network:** %s\n\n", t.network))
	sb.WriteString(fmt.Sprintf("To confirm and broadcast, call send again with confirmation_token: %q\n", token))
	sb.WriteString("Token expires in 5 minutes.\n")

	return ConfirmationResult(sb.String())
}

// confirmSend broadcasts a previously confirmed send transaction.
func (t *BitcoinTool) confirmSend(ctx context.Context, token string) *ToolResult {
	t.mu.Lock()
	ps, valid := t.pendingTokens[token]
	if valid {
		delete(t.pendingTokens, token)
	}
	t.mu.Unlock()

	if !valid {
		return ErrorResult("Invalid or expired confirmation token. Please initiate a new send.")
	}
	if time.Now().After(ps.Expires) {
		return ErrorResult("Confirmation token has expired. Please initiate a new send.")
	}

	// Load wallet
	w, err := t.loadWallet()
	if err != nil {
		return ErrorResult(err.Error())
	}

	keyMap, err := w.addressKeyMap()
	if err != nil {
		return ErrorResult(fmt.Sprintf("derive keys: %v", err))
	}

	// Fetch UTXOs for all wallet addresses
	var walletUTXOs []walletUTXO
	for addr := range keyMap {
		body, err := t.mempoolGet(ctx, "/address/"+addr+"/utxo")
		if err != nil {
			continue
		}

		var utxos []struct {
			TXID   string `json:"txid"`
			Vout   uint32 `json:"vout"`
			Value  int64  `json:"value"`
			Status struct {
				Confirmed bool `json:"confirmed"`
			} `json:"status"`
		}
		if err := json.Unmarshal(body, &utxos); err != nil {
			continue
		}

		for _, u := range utxos {
			if u.Status.Confirmed {
				walletUTXOs = append(walletUTXOs, walletUTXO{
					TxID:    u.TXID,
					Vout:    u.Vout,
					Address: addr,
					Value:   u.Value,
				})
			}
		}
	}

	if len(walletUTXOs) == 0 {
		return ErrorResult("no confirmed UTXOs available in wallet")
	}

	// Estimate tx size: ~68 vB per input + ~31 vB per output + ~11 vB overhead
	// Start with 2 outputs (destination + change)
	estimateVSize := func(nInputs int) int64 {
		return int64(nInputs)*68 + 2*31 + 11
	}

	// Coin selection: greedy, largest-first
	sortUTXOs(walletUTXOs)

	var selectedUTXOs []walletUTXO
	var selectedTotal int64
	for _, u := range walletUTXOs {
		selectedUTXOs = append(selectedUTXOs, u)
		selectedTotal += u.Value
		estimatedFee := estimateVSize(len(selectedUTXOs)) * ps.FeeRate
		if selectedTotal >= ps.AmountSats+estimatedFee {
			break
		}
	}

	estimatedFee := estimateVSize(len(selectedUTXOs)) * ps.FeeRate
	if selectedTotal < ps.AmountSats+estimatedFee {
		return ErrorResult(fmt.Sprintf(
			"insufficient funds: need %d sats (amount) + %d sats (est. fee) = %d sats, have %d sats",
			ps.AmountSats, estimatedFee, ps.AmountSats+estimatedFee, selectedTotal,
		))
	}

	// Build transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs
	for _, u := range selectedUTXOs {
		hash, err := chainhash.NewHashFromStr(u.TxID)
		if err != nil {
			return ErrorResult(fmt.Sprintf("invalid txid %s: %v", u.TxID, err))
		}
		outpoint := wire.NewOutPoint(hash, u.Vout)
		tx.AddTxIn(wire.NewTxIn(outpoint, nil, nil))
	}

	// Add destination output
	destScript, err := addressToScript(ps.ToAddr, w.netParams)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid destination address: %v", err))
	}
	tx.AddTxOut(wire.NewTxOut(ps.AmountSats, destScript))

	// Add change output if needed
	changeSats := selectedTotal - ps.AmountSats - estimatedFee
	if changeSats > 546 { // dust threshold
		changeAddr, err := w.nextChangeAddress()
		if err != nil {
			return ErrorResult(fmt.Sprintf("derive change address: %v", err))
		}
		changeScript, err := addressToScript(changeAddr, w.netParams)
		if err != nil {
			return ErrorResult(fmt.Sprintf("change address script: %v", err))
		}
		tx.AddTxOut(wire.NewTxOut(changeSats, changeScript))

		if err := w.save(); err != nil {
			return ErrorResult(fmt.Sprintf("save wallet: %v", err))
		}
	}

	// Sign all inputs
	if err := w.signTx(tx, selectedUTXOs, keyMap); err != nil {
		return ErrorResult(fmt.Sprintf("sign transaction: %v", err))
	}

	// Serialize
	var txBuf bytes.Buffer
	if err := tx.Serialize(&txBuf); err != nil {
		return ErrorResult(fmt.Sprintf("serialize tx: %v", err))
	}
	rawTxHex := hex.EncodeToString(txBuf.Bytes())

	// Broadcast via mempool.space
	respBody, err := t.mempoolPost(ctx, "/tx", []byte(rawTxHex))
	if err != nil {
		return ErrorResult(fmt.Sprintf("broadcast failed: %v", err))
	}

	txid := strings.TrimSpace(string(respBody))

	var sb strings.Builder
	sb.WriteString("**Bitcoin sent!**\n\n")
	sb.WriteString(fmt.Sprintf("**To:** `%s`\n", ps.ToAddr))
	sb.WriteString(fmt.Sprintf("**Amount:** %s BTC (%d sats)\n", ps.AmountBTC, ps.AmountSats))
	sb.WriteString(fmt.Sprintf("**Fee:** %d sats (~%d sat/vB)\n", estimatedFee, ps.FeeRate))
	sb.WriteString(fmt.Sprintf("**TXID:** `%s`\n", txid))
	sb.WriteString(fmt.Sprintf("**Inputs used:** %d | **Change:** %d sats\n", len(selectedUTXOs), changeSats))
	sb.WriteString(fmt.Sprintf("**Network:** %s\n", t.network))

	return NewToolResult(sb.String())
}

// ── Helpers ──────────────────────────────────────────────────────────

func satsToBTC(sats int64) string {
	btc := float64(sats) / 100_000_000
	return fmt.Sprintf("%.8f", btc)
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// sortUTXOs sorts UTXOs by value descending (largest first for coin selection).
func sortUTXOs(utxos []walletUTXO) {
	for i := 1; i < len(utxos); i++ {
		for j := i; j > 0 && utxos[j].Value > utxos[j-1].Value; j-- {
			utxos[j], utxos[j-1] = utxos[j-1], utxos[j]
		}
	}
}

// addressToScript converts a Bitcoin address to an output script.
func addressToScript(addr string, params *chaincfg.Params) ([]byte, error) {
	decoded, err := btcutil.DecodeAddress(addr, params)
	if err != nil {
		return nil, fmt.Errorf("decode address %q: %w", addr, err)
	}
	return txscript.PayToAddrScript(decoded)
}
