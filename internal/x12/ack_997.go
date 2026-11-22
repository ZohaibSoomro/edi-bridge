package x12

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var ack997Sequence atomic.Uint64

func nextAckControlNumbers() (isaCtrl string, gsCtrl string, stCtrl string) {
	seq := ack997Sequence.Add(1)
	isaCtrl = fmt.Sprintf("%09d", seq)
	gsCtrl = fmt.Sprintf("%d", seq)
	stCtrl = fmt.Sprintf("%04d", seq%10000)
	if stCtrl == "0000" {
		stCtrl = "0001"
	}
	return isaCtrl, gsCtrl, stCtrl
}

func functionalGroupForTx(txType string, fallback string) string {
	switch txType {
	case "850":
		return "PO"
	case "855":
		return "PR"
	case "810":
		return "IN"
	case "214":
		return "QM"
	case "856":
		return "SH"
	default:
		if fallback != "" {
			return fallback
		}
		return "ZZ"
	}
}

// Generate997 builds a compliant X12 997 with separate AK1..AK9 loops per functional group.
func Generate997(meta InterchangeMeta, txSets []TransactionSet, delims Delimiters, envelopeHasErrors bool) string {
	now := time.Now().UTC()
	date6 := now.Format("060102")
	date8 := now.Format("20060102")
	time4 := now.Format("1504")

	isaCtrl, gsCtrl, stCtrl := nextAckControlNumbers()
	usageIndicator := strings.TrimSpace(strings.ToUpper(meta.UsageIndicator))
	if usageIndicator == "" {
		usageIndicator = "P"
	}

	senderQual := meta.ReceiverQual
	if senderQual == "" {
		senderQual = "ZZ"
	}
	receiverQual := meta.SenderQual
	if receiverQual == "" {
		receiverQual = "ZZ"
	}

	sender := fmt.Sprintf("%-15s", strings.TrimSpace(meta.ReceiverID))
	receiver := fmt.Sprintf("%-15s", strings.TrimSpace(meta.SenderID))
	e := delims.Element
	t := delims.Segment

	// Group transactions by Functional Group control number while preserving order
	type txGroup struct {
		funcID    string
		groupCtrl string
		txs       []TransactionSet
	}
	var groups []txGroup
	groupIdx := make(map[string]int)

	for _, tx := range txSets {
		fID := tx.FunctionalID
		if fID == "" {
			fID = functionalGroupForTx(tx.TransactionType, "")
		}
		key := fID + ":" + tx.GroupCtrl
		if idx, exists := groupIdx[key]; exists {
			groups[idx].txs = append(groups[idx].txs, tx)
		} else {
			groupIdx[key] = len(groups)
			groups = append(groups, txGroup{
				funcID:    fID,
				groupCtrl: tx.GroupCtrl,
				txs:       []TransactionSet{tx},
			})
		}
	}

	lines := []string{
		fmt.Sprintf("ISA%s00%s          %s00%s          %s%s%s%s%s%s%s%s%s%s%s%s%sU%s00401%s%s%s0%s%s%s%s",
			e, e, e, e, e, senderQual, e, sender, e, receiverQual, e, receiver, e, date6, e, time4, e, e, e, isaCtrl, e, e, usageIndicator, e, delims.SubElement),
		fmt.Sprintf("GS%sFA%s%s%s%s%s%s%s%s%s%s%sX%s004010",
			e, e, strings.TrimSpace(meta.ReceiverID), e, strings.TrimSpace(meta.SenderID), e, date8, e, time4, e, gsCtrl, e, e),
		fmt.Sprintf("ST%s997%s%s", e, e, stCtrl),
	}

	for _, grp := range groups {
		lines = append(lines, fmt.Sprintf("AK1%s%s%s%s", e, grp.funcID, e, grp.groupCtrl))
		acceptedCount := 0
		for _, tx := range grp.txs {
			txAck := "A"
			if tx.HasErrors {
				txAck = "R"
			} else {
				acceptedCount++
			}
			lines = append(lines,
				fmt.Sprintf("AK2%s%s%s%s", e, tx.TransactionType, e, tx.ControlNumber),
				fmt.Sprintf("AK5%s%s", e, txAck),
			)
		}

		groupAck := "A"
		if envelopeHasErrors || acceptedCount == 0 {
			groupAck = "R"
		} else if acceptedCount < len(grp.txs) {
			groupAck = "P"
		}

		totalTx := len(grp.txs)
		lines = append(lines,
			fmt.Sprintf("AK9%s%s%s%d%s%d%s%d", e, groupAck, e, totalTx, e, totalTx, e, acceptedCount),
		)
	}

	seCount := len(lines[2:]) + 1
	lines = append(lines,
		fmt.Sprintf("SE%s%d%s%s", e, seCount, e, stCtrl),
		fmt.Sprintf("GE%s1%s%s", e, e, gsCtrl),
		fmt.Sprintf("IEA%s1%s%s", e, e, isaCtrl),
	)

	return strings.Join(lines, t) + t
}
