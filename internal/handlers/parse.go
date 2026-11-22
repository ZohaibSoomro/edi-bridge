package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/ZohaibSoomro/edi-bridge/internal/x12"
)

type ParseRequest struct {
	Payload     string `json:"payload"`
	Generate997 bool   `json:"generate_997"`
}

type ParseResponse struct {
	Success          bool                    `json:"success"`
	Interchange      x12.InterchangeMeta     `json:"interchange"`
	TransactionCount int                     `json:"transaction_count"`
	Transactions     []x12.ParsedTransaction `json:"transactions"`
	ValidationErrors []x12.ValidationError   `json:"validation_errors"`
	Ack997           string                  `json:"ack_997,omitempty"`
}

func HandleParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	var req ParseRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Payload == "" {
		http.Error(w, `{"error":"invalid JSON body; expected {\"payload\": \"ISA*...\"}"}`, http.StatusBadRequest)
		return
	}

	meta, txSets, delims, valErrors, err := x12.TokenizeAndSplit(req.Payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(ParseResponse{
			Success:      false,
			Transactions: make([]x12.ParsedTransaction, 0),
			ValidationErrors: []x12.ValidationError{{
				SegmentID: "ISA",
				Position:  1,
				Code:      "INVALID_HEADER",
				Message:   err.Error(),
			}},
		})
		return
	}

	if len(txSets) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(ParseResponse{
			Success:      false,
			Interchange:  meta,
			Transactions: make([]x12.ParsedTransaction, 0),
			ValidationErrors: append(valErrors, x12.ValidationError{
				SegmentID: "ST",
				Position:  1,
				Code:      "MISSING_TRANSACTION",
				Message:   "no ST..SE transaction sets found inside interchange",
			}),
		})
		return
	}

	parsedTxs := make([]x12.ParsedTransaction, 0, len(txSets))
	for i := range txSets {
		parsed, parseErr := x12.ParseTransaction(txSets[i], delims)
		if parseErr != nil {
			txSets[i].HasErrors = true
			valErrors = append(valErrors, x12.ValidationError{
				SegmentID: "ST",
				Position:  txSets[i].Segments[0].Position,
				Code:      "UNSUPPORTED_TX_TYPE",
				Message:   parseErr.Error(),
			})
			continue
		}
		parsedTxs = append(parsedTxs, parsed)
	}

	hasErrors := len(valErrors) > 0
	resp := ParseResponse{
		Success:          !hasErrors,
		Interchange:      meta,
		TransactionCount: len(parsedTxs),
		Transactions:     parsedTxs,
		ValidationErrors: valErrors,
	}

	if req.Generate997 {
		resp.Ack997 = x12.Generate997(meta, txSets, delims, hasErrors)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
