package x12

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Delimiters struct {
	Element    string
	SubElement string
	Segment    string
}

type Segment struct {
	ID       string   `json:"id"`
	Elements []string `json:"elements"`
	Position int      `json:"position"`
}

type ValidationError struct {
	SegmentID string `json:"segment_id"`
	Position  int    `json:"position"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type FunctionalGroup struct {
	FunctionalID    string `json:"functional_id"`
	GroupSenderID   string `json:"group_sender_id"`
	GroupReceiverID string `json:"group_receiver_id"`
	GroupCtrl       string `json:"group_control_number"`
	Version         string `json:"version"`
	TxCount         int    `json:"transaction_count"`
}

type InterchangeMeta struct {
	SenderQual       string            `json:"sender_qualifier"`
	SenderID         string            `json:"sender_id"`
	ReceiverQual     string            `json:"receiver_qualifier"`
	ReceiverID       string            `json:"receiver_id"`
	InterchangeDate  string            `json:"interchange_date"`
	InterchangeTime  string            `json:"interchange_time"`
	InterchangeCtrl  string            `json:"interchange_control_number"`
	UsageIndicator   string            `json:"usage_indicator,omitempty"`
	FunctionalGroups []FunctionalGroup `json:"functional_groups"`
}

type TransactionSet struct {
	FunctionalID    string    `json:"functional_id"`
	GroupCtrl       string    `json:"group_control_number"`
	TransactionType string    `json:"transaction_type"`
	ControlNumber   string    `json:"control_number"`
	Segments        []Segment `json:"-"`
	HasErrors       bool      `json:"has_errors"`
}

func normalizeCtrl(s string) string {
	trimmed := strings.TrimLeft(strings.TrimSpace(s), "0")
	if trimmed == "" && strings.TrimSpace(s) != "" {
		return "0"
	}
	return trimmed
}

func safeElem(elems []string, idx int) string {
	if idx < 0 || idx >= len(elems) {
		return ""
	}
	return strings.TrimSpace(elems[idx])
}

func splitSubElem(val, subSep string) string {
	if val == "" || subSep == "" {
		return val
	}
	parts := strings.Split(val, subSep)
	return strings.TrimSpace(parts[0])
}

func ExtractDelimiters(raw string) (Delimiters, error) {
	cleaned := strings.TrimSpace(raw)
	if len(cleaned) < 106 || !strings.HasPrefix(cleaned, "ISA") {
		return Delimiters{}, errors.New("invalid X12 payload: missing or truncated 106-character ISA header")
	}
	return Delimiters{
		Element:    string(cleaned[3]),
		SubElement: string(cleaned[104]),
		Segment:    string(cleaned[105]),
	}, nil
}

func TokenizeAndSplit(raw string) (InterchangeMeta, []TransactionSet, Delimiters, []ValidationError, error) {
	valErrors := make([]ValidationError, 0)
	delims, err := ExtractDelimiters(raw)
	if err != nil {
		return InterchangeMeta{}, nil, delims, valErrors, err
	}

	normalized := strings.TrimSpace(raw)
	if delims.Segment != "\n" && delims.Segment != "\r" {
		normalized = strings.ReplaceAll(normalized, "\r\n", "")
		normalized = strings.ReplaceAll(normalized, "\n", "")
	}

	rawSegments := strings.Split(normalized, delims.Segment)
	var segments []Segment
	for _, s := range rawSegments {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		parts := strings.Split(s, delims.Element)
		segments = append(segments, Segment{
			ID:       parts[0],
			Elements: parts[1:],
			Position: len(segments) + 1,
		})
	}

	meta := InterchangeMeta{
		FunctionalGroups: make([]FunctionalGroup, 0),
	}
	var txSets []TransactionSet
	var currentGroup *FunctionalGroup
	var currentTx *TransactionSet
	var stStartIdx int

	for i, seg := range segments {
		switch seg.ID {
		case "ISA":
			meta.SenderQual = safeElem(seg.Elements, 4)
			meta.SenderID = safeElem(seg.Elements, 5)
			meta.ReceiverQual = safeElem(seg.Elements, 6)
			meta.ReceiverID = safeElem(seg.Elements, 7)
			meta.InterchangeDate = safeElem(seg.Elements, 8)
			meta.InterchangeTime = safeElem(seg.Elements, 9)
			meta.InterchangeCtrl = safeElem(seg.Elements, 12)
			meta.UsageIndicator = strings.TrimSpace(strings.ToUpper(safeElem(seg.Elements, 14)))

		case "IEA":
			expectedGroups, _ := strconv.Atoi(safeElem(seg.Elements, 0))
			ieaCtrl := safeElem(seg.Elements, 1)
			if expectedGroups != len(meta.FunctionalGroups) {
				valErrors = append(valErrors, ValidationError{
					SegmentID: "IEA",
					Position:  seg.Position,
					Code:      "IEA_COUNT_MISMATCH",
					Message:   fmt.Sprintf("IEA01 group count (%d) does not match actual GS count (%d)", expectedGroups, len(meta.FunctionalGroups)),
				})
			}
			if meta.InterchangeCtrl != "" && normalizeCtrl(ieaCtrl) != normalizeCtrl(meta.InterchangeCtrl) {
				valErrors = append(valErrors, ValidationError{
					SegmentID: "IEA",
					Position:  seg.Position,
					Code:      "ENV_ISA_MISMATCH",
					Message:   fmt.Sprintf("IEA02 control number (%s) does not match ISA13 (%s)", ieaCtrl, meta.InterchangeCtrl),
				})
			}

		case "GS":
			currentGroup = &FunctionalGroup{
				FunctionalID:    safeElem(seg.Elements, 0),
				GroupSenderID:   safeElem(seg.Elements, 1),
				GroupReceiverID: safeElem(seg.Elements, 2),
				GroupCtrl:       safeElem(seg.Elements, 5),
				Version:         safeElem(seg.Elements, 7),
			}

		case "GE":
			if currentGroup != nil {
				expectedTxs, _ := strconv.Atoi(safeElem(seg.Elements, 0))
				geCtrl := safeElem(seg.Elements, 1)
				if expectedTxs != currentGroup.TxCount {
					valErrors = append(valErrors, ValidationError{
						SegmentID: "GE",
						Position:  seg.Position,
						Code:      "GE_COUNT_MISMATCH",
						Message:   fmt.Sprintf("GE01 transaction count (%d) does not match actual ST count (%d) in GS %s", expectedTxs, currentGroup.TxCount, currentGroup.GroupCtrl),
					})
				}
				if normalizeCtrl(geCtrl) != normalizeCtrl(currentGroup.GroupCtrl) {
					valErrors = append(valErrors, ValidationError{
						SegmentID: "GE",
						Position:  seg.Position,
						Code:      "ENV_GS_MISMATCH",
						Message:   fmt.Sprintf("GE02 control number (%s) does not match GS06 (%s)", geCtrl, currentGroup.GroupCtrl),
					})
				}
				meta.FunctionalGroups = append(meta.FunctionalGroups, *currentGroup)
				currentGroup = nil
			}

		case "ST":
			stStartIdx = i
			var fID, gCtrl string
			if currentGroup != nil {
				currentGroup.TxCount++
				fID = currentGroup.FunctionalID
				gCtrl = currentGroup.GroupCtrl
			}
			currentTx = &TransactionSet{
				FunctionalID:    fID,
				GroupCtrl:       gCtrl,
				TransactionType: safeElem(seg.Elements, 0),
				ControlNumber:   safeElem(seg.Elements, 1),
				Segments:        []Segment{seg},
			}

		case "SE":
			if currentTx != nil {
				currentTx.Segments = append(currentTx.Segments, seg)
				expectedCount, _ := strconv.Atoi(safeElem(seg.Elements, 0))
				actualCount := (i - stStartIdx) + 1
				seCtrl := safeElem(seg.Elements, 1)

				if expectedCount != actualCount {
					currentTx.HasErrors = true
					valErrors = append(valErrors, ValidationError{
						SegmentID: "SE",
						Position:  seg.Position,
						Code:      "SE_COUNT_MISMATCH",
						Message:   fmt.Sprintf("SE01 segment count (%d) does not match actual count (%d) in ST %s", expectedCount, actualCount, currentTx.ControlNumber),
					})
				}
				if normalizeCtrl(seCtrl) != normalizeCtrl(currentTx.ControlNumber) {
					currentTx.HasErrors = true
					valErrors = append(valErrors, ValidationError{
						SegmentID: "SE",
						Position:  seg.Position,
						Code:      "ENV_ST_MISMATCH",
						Message:   fmt.Sprintf("SE02 control number (%s) does not match ST02 (%s)", seCtrl, currentTx.ControlNumber),
					})
				}
				txSets = append(txSets, *currentTx)
				currentTx = nil
			}

		default:
			if currentTx != nil {
				currentTx.Segments = append(currentTx.Segments, seg)
			}
		}

	}

	if currentTx != nil {
		currentTx.HasErrors = true
		valErrors = append(valErrors, ValidationError{
			SegmentID: "ST",
			Position:  currentTx.Segments[0].Position,
			Code:      "UNCLOSED_TRANSACTION",
			Message:   fmt.Sprintf("Transaction ST %s is missing a closing SE segment", currentTx.ControlNumber),
		})
		txSets = append(txSets, *currentTx)
	}

	return meta, txSets, delims, valErrors, nil
}
