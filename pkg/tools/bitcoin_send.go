package tools

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

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
	fmt.Fprintf(&sb, "**To:** `%s`\n", toAddr)
	fmt.Fprintf(&sb, "**Amount:** %s BTC (%d sats)\n", amountStr, amountSats)
	fmt.Fprintf(&sb, "**Fee rate:** ~%d sat/vB\n", feeRateSatVB)
	fmt.Fprintf(&sb, "**Network:** %s\n\n", t.network)
	fmt.Fprintf(&sb, "To confirm and broadcast, call send again with confirmation_token: %q\n", token)
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
	fmt.Fprintf(&sb, "**To:** `%s`\n", ps.ToAddr)
	fmt.Fprintf(&sb, "**Amount:** %s BTC (%d sats)\n", ps.AmountBTC, ps.AmountSats)
	fmt.Fprintf(&sb, "**Fee:** %d sats (~%d sat/vB)\n", estimatedFee, ps.FeeRate)
	fmt.Fprintf(&sb, "**TXID:** `%s`\n", txid)
	fmt.Fprintf(&sb, "**Inputs used:** %d | **Change:** %d sats\n", len(selectedUTXOs), changeSats)
	fmt.Fprintf(&sb, "**Network:** %s\n", t.network)

	return NewToolResult(sb.String())
}

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
