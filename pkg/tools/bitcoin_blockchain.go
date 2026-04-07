package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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
	fmt.Fprintf(&sb, "**Address:** `%s`\n", addr)
	fmt.Fprintf(&sb, "**Network:** %s\n\n", t.network)
	fmt.Fprintf(&sb, "**Confirmed balance:** %s BTC (%d sats)\n", satsToBTC(confirmed), confirmed)
	if unconfirmed != 0 {
		fmt.Fprintf(&sb, "**Unconfirmed:** %s BTC (%d sats)\n", satsToBTC(unconfirmed), unconfirmed)
	}
	fmt.Fprintf(&sb, "**Total:** %s BTC\n", satsToBTC(confirmed+unconfirmed))
	fmt.Fprintf(&sb, "**Transactions:** %d confirmed", data.Chain.TxCount)
	if data.Mempool.TxCount > 0 {
		fmt.Fprintf(&sb, ", %d unconfirmed", data.Mempool.TxCount)
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
	fmt.Fprintf(&sb, "**Transactions for `%s`** (latest %d):\n\n", addr, min(len(txs), 25))

	for i, tx := range txs {
		if i >= 25 {
			fmt.Fprintf(&sb, "\n*... and %d more transactions*\n", len(txs)-25)
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

		fmt.Fprintf(&sb, "**%d.** %s %s BTC\n", i+1, direction, satsToBTC(abs64(net)))
		fmt.Fprintf(&sb, "   TXID: `%s`\n", tx.TXID[:16]+"...")
		fmt.Fprintf(&sb, "   %s", status)
		if timeStr != "" {
			fmt.Fprintf(&sb, " | %s", timeStr)
		}
		fmt.Fprintf(&sb, " | Fee: %d sats\n\n", tx.Fee)
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
	fmt.Fprintf(&sb, "**UTXOs for `%s`** (%d):\n\n", addr, len(utxos))
	sb.WriteString("| # | Value (BTC) | Value (sats) | Confirmed | TXID:vout |\n")
	sb.WriteString("|---|-------------|--------------|-----------|------------|\n")

	for i, u := range utxos {
		conf := "no"
		if u.Status.Confirmed {
			conf = "yes"
		}
		total += u.Value
		fmt.Fprintf(&sb, "| %d | %s | %d | %s | `%s:%d` |\n",
			i+1, satsToBTC(u.Value), u.Value, conf, u.TXID[:12]+"...", u.Vout)
	}
	fmt.Fprintf(&sb, "\n**Total:** %s BTC (%d sats)\n", satsToBTC(total), total)

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
	fmt.Fprintf(&sb, "**Transaction:** `%s`\n\n", tx.TXID)

	if tx.Status.Confirmed {
		timeStr := time.Unix(tx.Status.BlockTime, 0).Format("2006-01-02 15:04 UTC")
		fmt.Fprintf(&sb, "**Status:** Confirmed at block %d\n", tx.Status.BlockHeight)
		fmt.Fprintf(&sb, "**Time:** %s\n", timeStr)
	} else {
		sb.WriteString("**Status:** Unconfirmed (in mempool)\n")
	}

	fmt.Fprintf(&sb, "**Size:** %d vB | **Fee:** %d sats (%.1f sat/vB)\n\n", vsize, tx.Fee, feeRate)

	fmt.Fprintf(&sb, "**Inputs (%d):** %s BTC\n", len(tx.VIn), satsToBTC(totalIn))
	for i, vin := range tx.VIn {
		fmt.Fprintf(&sb, "  %d. `%s` — %s BTC\n", i+1, vin.Prevout.ScriptPubKeyAddr, satsToBTC(vin.Prevout.Value))
	}

	fmt.Fprintf(&sb, "\n**Outputs (%d):** %s BTC\n", len(tx.VOut), satsToBTC(totalOut))
	for i, vout := range tx.VOut {
		fmt.Fprintf(&sb, "  %d. `%s` — %s BTC\n", i+1, vout.ScriptPubKeyAddr, satsToBTC(vout.Value))
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
	fmt.Fprintf(&sb, "| Fastest (~10 min) | %d sat/vB | %d sats |\n", fees.FastestFee, fees.FastestFee*140)
	fmt.Fprintf(&sb, "| Half hour | %d sat/vB | %d sats |\n", fees.HalfHourFee, fees.HalfHourFee*140)
	fmt.Fprintf(&sb, "| Hour | %d sat/vB | %d sats |\n", fees.HourFee, fees.HourFee*140)
	fmt.Fprintf(&sb, "| Economy | %d sat/vB | %d sats |\n", fees.EconomyFee, fees.EconomyFee*140)
	fmt.Fprintf(&sb, "| Minimum | %d sat/vB | %d sats |\n", fees.MinimumFee, fees.MinimumFee*140)
	fmt.Fprintf(&sb, "\n**Network:** %s\n", t.network)

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
	fmt.Fprintf(&sb, "| USD | $%.2f |\n", prices.USD)
	fmt.Fprintf(&sb, "| EUR | %.2f |\n", prices.EUR)
	fmt.Fprintf(&sb, "| GBP | %.2f |\n", prices.GBP)
	fmt.Fprintf(&sb, "| CHF | CHF %.2f |\n", prices.CHF)
	fmt.Fprintf(&sb, "| CAD | C$%.2f |\n", prices.CAD)
	fmt.Fprintf(&sb, "| AUD | A$%.2f |\n", prices.AUD)
	fmt.Fprintf(&sb, "| JPY | %.0f |\n", prices.JPY)

	if prices.USD > 0 {
		satsPerDollar := 100_000_000 / prices.USD
		fmt.Fprintf(&sb, "\n**1 USD = %.0f sats**\n", satsPerDollar)
	}

	return NewToolResult(sb.String())
}
