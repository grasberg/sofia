package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

	// Write mnemonic to ~/.sofia/wallet_recovery.txt (never next to the binary).
	home, _ := os.UserHomeDir()
	recoveryPath := filepath.Join(home, ".sofia", "wallet_recovery.txt")
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
	fmt.Fprintf(&sb, "**Recovery phrase saved to:** `%s`\n", recoveryPath)
	sb.WriteString("**IMPORTANT:** Open that file to back up your recovery phrase, then delete it.\n")
	sb.WriteString("The recovery phrase is the ONLY way to recover your wallet. Never share it.\n\n")
	fmt.Fprintf(&sb, "**First receive address:** `%s`\n", addr)
	fmt.Fprintf(&sb, "**Network:** %s\n", t.network)
	fmt.Fprintf(&sb, "**Wallet file:** `%s`\n", t.walletPath)

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
	fmt.Fprintf(&sb, "**First receive address:** `%s`\n", addr)
	fmt.Fprintf(&sb, "**Network:** %s\n", t.network)
	fmt.Fprintf(&sb, "**Wallet file:** `%s`\n", t.walletPath)

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
	fmt.Fprintf(&sb, "**New receive address:** `%s`\n", addr)
	fmt.Fprintf(&sb, "**Network:** %s\n", t.network)
	fmt.Fprintf(&sb, "**Address index:** %d\n", w.nextReceive-1)

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
	fmt.Fprintf(&sb, "**Wallet addresses** (%d):\n\n", len(addrs))

	receiveCount := int(w.nextReceive) + 1
	for i, addr := range addrs {
		if i < receiveCount {
			fmt.Fprintf(&sb, "  %d. `%s` (receive #%d)\n", i+1, addr, i)
		} else {
			fmt.Fprintf(&sb, "  %d. `%s` (change #%d)\n", i+1, addr, i-receiveCount)
		}
	}

	fmt.Fprintf(&sb, "\n**Network:** %s\n", t.network)

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
	fmt.Fprintf(&sb, "**Confirmed:** %s BTC (%d sats)\n", satsToBTC(totalConfirmed), totalConfirmed)
	if totalUnconfirmed != 0 {
		fmt.Fprintf(&sb, "**Unconfirmed:** %s BTC (%d sats)\n", satsToBTC(totalUnconfirmed), totalUnconfirmed)
	}
	fmt.Fprintf(&sb, "**Total:** %s BTC\n", satsToBTC(totalConfirmed+totalUnconfirmed))
	fmt.Fprintf(&sb, "**Addresses scanned:** %d\n", len(addrs))
	fmt.Fprintf(&sb, "**Network:** %s\n", t.network)

	return NewToolResult(sb.String())
}
