package x12

import (
	"strings"
	"testing"
)

func TestCustomDelimitersAndUsageIndicator(t *testing.T) {
	// Pipe '|' element separator, '^' sub-element separator, and newline segment terminator
	raw := strings.Join([]string{
		"ISA|00|          |00|          |ZZ|SENDER123      |ZZ|RECEIVER456    |261122|0245|U|00401|000000205|0|T|^",
		"GS|PO|SENDER123|RECEIVER456|20261122|0245|205|X|004010",
		"ST|850|0001",
		"BEG|00|NE|PO-CUSTOM-01||20261122",
		"PO1|1|3|EA^CASE|19.99||VP|SKU-PIPE",
		"CTT|1",
		"SE|5|0001",
		"GE|1|205",
		"IEA|1|000000205",
	}, "\n") + "\n"

	meta, txs, delims, valErrs, err := TokenizeAndSplit(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing custom delimiter envelope: %v", err)
	}
	if len(valErrs) != 0 {
		t.Fatalf("expected 0 validation errors, got %+v", valErrs)
	}
	if delims.Element != "|" || delims.SubElement != "^" || delims.Segment != "\n" {
		t.Errorf("unexpected detected delimiters: %+v", delims)
	}
	if meta.UsageIndicator != "T" {
		t.Errorf("expected UsageIndicator=T, got %q", meta.UsageIndicator)
	}
	if len(txs) != 1 || txs[0].TransactionType != "850" {
		t.Fatalf("expected 1 parsed 850 transaction, got %+v", txs)
	}

	doc, err := ParseTransaction(txs[0], delims)
	if err != nil {
		t.Fatalf("unexpected ParseTransaction error: %v", err)
	}
	if doc.PONumber != "PO-CUSTOM-01" {
		t.Errorf("expected PO-CUSTOM-01, got %q", doc.PONumber)
	}
	if len(doc.LineItems) != 1 || doc.LineItems[0].UnitOfMeas != "EA" {
		t.Errorf("expected sub-element stripped UOM 'EA', got %+v", doc.LineItems)
	}

	// Outbound 997 should honor custom delimiters and echo UsageIndicator 'T'
	ack := Generate997(meta, txs, delims, false)
	if strings.Contains(ack, "\r") {
		t.Errorf("expected no CR characters in 997, got %q", ack)
	}
	if !strings.Contains(ack, "|0|T|^") {
		t.Errorf("expected 997 ISA to use '|' and '^' with UsageIndicator 'T', got %q", ack)
	}
}

func TestSEValidationErrors(t *testing.T) {
	// SE01 says 99 segments instead of actual count, and SE02 says 9999 instead of 0001
	raw := "ISA*00*          *00*          *ZZ*SENDER123      *ZZ*RECEIVER456    *261122*0245*U*00401*000000101*0*P*:~" +
		"GS*PO*SENDER123*RECEIVER456*20261122*0245*101*X*004010~" +
		"ST*850*0001~" +
		"BEG*00*NE*PO-BAD-SE**20261122~" +
		"PO1*1*2*EA*10.00**VP*ITEM-1~" +
		"SE*99*9999~" +
		"GE*1*101~" +
		"IEA*1*000000101~"

	meta, txs, delims, valErrs, err := TokenizeAndSplit(raw)
	if err != nil {
		t.Fatalf("unexpected envelope parse error: %v", err)
	}
	if !hasValidationCode(valErrs, "SE_COUNT_MISMATCH") {
		t.Errorf("expected SE_COUNT_MISMATCH in validation errors, got %+v", valErrs)
	}
	if !hasValidationCode(valErrs, "ENV_ST_MISMATCH") {
		t.Errorf("expected ENV_ST_MISMATCH in validation errors, got %+v", valErrs)
	}
	if len(txs) != 1 || !txs[0].HasErrors {
		t.Fatalf("expected transaction HasErrors=true on SE mismatch, got %+v", txs)
	}

	ack := Generate997(meta, txs, delims, false)
	if !strings.Contains(ack, "AK5*R") {
		t.Errorf("expected rejected transaction AK5*R* in 997, got %q", ack)
	}
}

func TestGEValidationErrors(t *testing.T) {
	// GE01 says 5 transactions instead of 1, and GE02 says 888 instead of 101
	raw := "ISA*00*          *00*          *ZZ*SENDER123      *ZZ*RECEIVER456    *261122*0245*U*00401*000000101*0*P*:~" +
		"GS*PO*SENDER123*RECEIVER456*20261122*0245*101*X*004010~" +
		"ST*850*0001~" +
		"BEG*00*NE*PO-BAD-GE**20261122~" +
		"PO1*1*2*EA*10.00**VP*ITEM-1~" +
		"SE*4*0001~" +
		"GE*5*888~" +
		"IEA*1*000000101~"

	_, _, _, valErrs, err := TokenizeAndSplit(raw)
	if err != nil {
		t.Fatalf("unexpected envelope parse error: %v", err)
	}
	if !hasValidationCode(valErrs, "GE_COUNT_MISMATCH") {
		t.Errorf("expected GE_COUNT_MISMATCH in validation errors, got %+v", valErrs)
	}
	if !hasValidationCode(valErrs, "ENV_GS_MISMATCH") {
		t.Errorf("expected ENV_GS_MISMATCH in validation errors, got %+v", valErrs)
	}
}

func TestIEAValidationErrors(t *testing.T) {
	// IEA01 says 2 functional groups instead of 1, and IEA02 says 000000999 instead of 000000101
	raw := "ISA*00*          *00*          *ZZ*SENDER123      *ZZ*RECEIVER456    *261122*0245*U*00401*000000101*0*P*:~" +
		"GS*PO*SENDER123*RECEIVER456*20261122*0245*101*X*004010~" +
		"ST*850*0001~" +
		"BEG*00*NE*PO-BAD-IEA**20261122~" +
		"PO1*1*2*EA*10.00**VP*ITEM-1~" +
		"SE*4*0001~" +
		"GE*1*101~" +
		"IEA*2*000000999~"

	meta, txs, delims, valErrs, err := TokenizeAndSplit(raw)
	if err != nil {
		t.Fatalf("unexpected envelope parse error: %v", err)
	}
	if !hasValidationCode(valErrs, "IEA_COUNT_MISMATCH") {
		t.Errorf("expected IEA_COUNT_MISMATCH in validation errors, got %+v", valErrs)
	}
	if !hasValidationCode(valErrs, "ENV_ISA_MISMATCH") {
		t.Errorf("expected ENV_ISA_MISMATCH in validation errors, got %+v", valErrs)
	}

	ack := Generate997(meta, txs, delims, true)
	if !strings.Contains(ack, "AK9*R*") {
		t.Errorf("expected rejected group AK9*R* when envelopeHasErrors=true, got %q", ack)
	}
}

func hasValidationCode(errs []ValidationError, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}
