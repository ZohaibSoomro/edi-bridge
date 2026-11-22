package x12

import (
	"strings"
	"testing"
)

func testDelims() Delimiters {
	return Delimiters{
		Element:    "*",
		SubElement: ":",
		Segment:    "~",
	}
}

func TestParseTransaction850PurchaseOrder(t *testing.T) {
	tx := TransactionSet{
		FunctionalID:    "PO",
		GroupCtrl:       "101",
		TransactionType: "850",
		ControlNumber:   "0001",
		Segments: []Segment{
			{ID: "BEG", Elements: []string{"00", "NE", "PO-998877", "", "20261122"}},
			{ID: "DTM", Elements: []string{"002", "20261128"}},
			{ID: "N1", Elements: []string{"ST", "Acme Warehouse", "92", "WH01"}},
			{ID: "N3", Elements: []string{"120 Industrial Pkwy"}},
			{ID: "N4", Elements: []string{"Austin", "TX", "78701", "US"}},
			{ID: "PO1", Elements: []string{"1", "50", "EA", "24.50", "", "VP", "SKU-1001", "UP", "012345678905"}},
			{ID: "PID", Elements: []string{"F", "", "", "", "Wireless Barcode Scanner"}},
			{ID: "CTT", Elements: []string{"1"}},
		},
	}

	doc, err := ParseTransaction(tx, testDelims())
	if err != nil {
		t.Fatalf("unexpected error parsing 850: %v", err)
	}
	if doc.TransactionType != "850" || doc.PONumber != "PO-998877" {
		t.Errorf("unexpected 850 header fields: %+v", doc)
	}
	if doc.TotalLineItems != 1 || len(doc.LineItems) != 1 {
		t.Fatalf("expected 1 line item, got total=%d len=%d", doc.TotalLineItems, len(doc.LineItems))
	}
	if doc.LineItems[0].ProductIDs["VP"] != "SKU-1001" {
		t.Errorf("expected VP=SKU-1001, got %+v", doc.LineItems[0].ProductIDs)
	}
	if len(doc.Parties) != 1 || doc.Parties[0].City != "Austin" {
		t.Errorf("expected party in Austin, got %+v", doc.Parties)
	}
}

func TestParseTransaction855810And214(t *testing.T) {
	// 855 PO Acknowledgment
	tx855 := TransactionSet{
		FunctionalID:    "PR",
		GroupCtrl:       "201",
		TransactionType: "855",
		ControlNumber:   "0001",
		Segments: []Segment{
			{ID: "BAK", Elements: []string{"00", "AC", "PO-5001", "20261120", "", "", "", "ACK-5001", "20261121"}},
			{ID: "PO1", Elements: []string{"1", "10", "EA", "15.00", "", "VP", "PART-10"}},
			{ID: "ACK", Elements: []string{"AC", "10"}},
			{ID: "SE", Elements: []string{"4", "0001"}},
		},
	}
	doc855, err := ParseTransaction(tx855, testDelims())
	if err != nil {
		t.Fatalf("unexpected error parsing 855: %v", err)
	}
	if doc855.PONumber != "PO-5001" || doc855.DocumentNumber != "ACK-5001" {
		t.Errorf("unexpected 855 doc/po numbers: %+v", doc855)
	}
	if len(doc855.LineItems) != 1 || doc855.LineItems[0].AckStatus != "AC" || doc855.LineItems[0].AckQuantity != 10 {
		t.Errorf("unexpected 855 ack item: %+v", doc855.LineItems)
	}

	// 810 Invoice with implied-cent TDS
	tx810 := TransactionSet{
		FunctionalID:    "IN",
		GroupCtrl:       "301",
		TransactionType: "810",
		ControlNumber:   "0001",
		Segments: []Segment{
			{ID: "BIG", Elements: []string{"20261122", "INV-7741", "20261120", "PO-998877"}},
			{ID: "IT1", Elements: []string{"1", "50", "EA", "24.50", "", "VP", "SKU-1001"}},
			{ID: "TDS", Elements: []string{"122500"}},
		},
	}
	doc810, err := ParseTransaction(tx810, testDelims())
	if err != nil {
		t.Fatalf("unexpected error parsing 810: %v", err)
	}
	if doc810.DocumentNumber != "INV-7741" || doc810.TotalAmount != 1225.00 {
		t.Errorf("expected INV-7741 and TotalAmount=1225.00, got %+v", doc810)
	}

	// 214 Shipment Status
	tx214 := TransactionSet{
		FunctionalID:    "QM",
		GroupCtrl:       "401",
		TransactionType: "214",
		ControlNumber:   "0001",
		Segments: []Segment{
			{ID: "B10", Elements: []string{"PRO-884422", "SHP-9001", "FEDX"}},
			{ID: "AT7", Elements: []string{"X6", "NS", "", "", "20261122", "0255", "CT"}},
			{ID: "MS1", Elements: []string{"Austin", "TX", "US"}},
			{ID: "SE", Elements: []string{"4", "0001"}},
		},
	}
	doc214, err := ParseTransaction(tx214, testDelims())
	if err != nil {
		t.Fatalf("unexpected error parsing 214: %v", err)
	}
	if doc214.ShipmentID != "SHP-9001" || doc214.CarrierSCAC != "FEDX" {
		t.Errorf("unexpected 214 header: %+v", doc214)
	}
	if len(doc214.ShipmentEvents) != 1 || doc214.ShipmentEvents[0].StatusCode != "X6" || doc214.ShipmentEvents[0].City != "Austin" {
		t.Errorf("unexpected 214 events: %+v", doc214.ShipmentEvents)
	}

	// Unsupported transaction type
	if _, err := ParseTransaction(TransactionSet{TransactionType: "999"}, testDelims()); err == nil {
		t.Errorf("expected error for unsupported transaction type 999")
	}
}

func TestParseTransaction856ASN(t *testing.T) {
	tx856 := TransactionSet{
		FunctionalID:    "SH",
		GroupCtrl:       "3281",
		TransactionType: "856",
		ControlNumber:   "8174",
		Segments: []Segment{
			{ID: "BSN", Elements: []string{"00", "14482080", "20261105", "1218"}},
			{ID: "HL", Elements: []string{"1", "", "S"}},
			{ID: "TD5", Elements: []string{"", "2", "XPO", "M", "XPO LOGISTICS"}},
			{ID: "N1", Elements: []string{"SF", "Emerson Professional Tools"}},
			{ID: "HL", Elements: []string{"2", "1", "O"}},
			{ID: "PRF", Elements: []string{"P002071700", "", "", "20261102"}},
			{ID: "HL", Elements: []string{"3", "2", "P"}},
			{ID: "MAN", Elements: []string{"GM", "00107833104820800010"}},
			{ID: "HL", Elements: []string{"4", "3", "I"}},
			{ID: "LIN", Elements: []string{"000006", "BP", "3241371", "UP", "783310392576", "VP", "39257"}},
			{ID: "SN1", Elements: []string{"000006", "1", "EA"}},
			{ID: "PID", Elements: []string{"F", "", "", "", "MANDREL-PLASTIC 4.0\""}},
		},
	}

	doc, err := ParseTransaction(tx856, testDelims())
	if err != nil {
		t.Fatalf("unexpected error parsing 856: %v", err)
	}
	if doc.ASN == nil || len(doc.ASN.Shipments) != 1 {
		t.Fatalf("expected hierarchical ASN shipment, got %+v", doc.ASN)
	}
	ship := doc.ASN.Shipments[0]
	if len(ship.Orders) != 1 || len(ship.Orders[0].Packs) != 1 || len(ship.Orders[0].Packs[0].Items) != 1 {
		t.Fatalf("unexpected S->O->P->I hierarchy: %+v", ship)
	}
	item := ship.Orders[0].Packs[0].Items[0]
	if item.ProductIDs["BP"] != "3241371" || item.ProductIDs["VP"] != "39257" {
		t.Errorf("expected BP=3241371 and VP=39257, got %+v", item.ProductIDs)
	}
}

func TestGenerate997UsageIndicatorAndUniqueControls(t *testing.T) {
	delims := testDelims()
	txs := []TransactionSet{
		{
			FunctionalID:    "PO",
			GroupCtrl:       "101",
			TransactionType: "850",
			ControlNumber:   "0001",
			HasErrors:       false,
		},
	}

	metaTest := InterchangeMeta{
		SenderQual:     "ZZ",
		SenderID:       "SENDER123",
		ReceiverQual:   "ZZ",
		ReceiverID:     "RECEIVER456",
		UsageIndicator: "T",
	}

	ack1 := Generate997(metaTest, txs, delims, false)
	ack2 := Generate997(metaTest, txs, delims, false)

	if strings.ContainsAny(ack1, "\r\n") || strings.ContainsAny(ack2, "\r\n") {
		t.Fatalf("expected newline-free 997 output, got ack1=%q ack2=%q", ack1, ack2)
	}
	if !strings.Contains(ack1, "*0*T*:~") {
		t.Errorf("expected ISA15=T in outbound 997, got %q", ack1)
	}
	if ack1 == ack2 {
		t.Errorf("expected consecutive 997s to have distinct control numbers, both produced %q", ack1)
	}
}
