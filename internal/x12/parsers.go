package x12

import (
	"fmt"
	"strconv"
)

type DateRef struct {
	Qualifier string `json:"qualifier"`
	Date      string `json:"date"`
	Time      string `json:"time,omitempty"`
}

type Party struct {
	EntityCode   string `json:"entity_code"`
	Name         string `json:"name"`
	IDQual       string `json:"id_qualifier,omitempty"`
	ID           string `json:"id,omitempty"`
	AddressLine1 string `json:"address_line_1,omitempty"`
	AddressLine2 string `json:"address_line_2,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	PostalCode   string `json:"postal_code,omitempty"`
	Country      string `json:"country,omitempty"`
}

type LineItem struct {
	LineNumber  string            `json:"line_number"`
	Quantity    float64           `json:"quantity"`
	UnitOfMeas  string            `json:"uom"`
	UnitPrice   float64           `json:"unit_price"`
	ProductIDs  map[string]string `json:"product_ids,omitempty"`
	Description string            `json:"description,omitempty"`
	AckStatus   string            `json:"ack_status,omitempty"`
	AckQuantity float64           `json:"ack_quantity,omitempty"`
	Dates       []DateRef         `json:"dates,omitempty"`
}

type ChargeAllowance struct {
	Indicator   string  `json:"indicator"`
	Code        string  `json:"code"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description,omitempty"`
}

type ShipmentEvent struct {
	StatusCode    string `json:"status_code"`
	ReasonCode    string `json:"reason_code,omitempty"`
	Date          string `json:"date"`
	Time          string `json:"time,omitempty"`
	TimeZone      string `json:"time_zone,omitempty"`
	City          string `json:"city,omitempty"`
	State         string `json:"state,omitempty"`
	Country       string `json:"country,omitempty"`
	EquipmentInit string `json:"equipment_initial,omitempty"`
	EquipmentNum  string `json:"equipment_number,omitempty"`
}

type ParsedTransaction struct {
	TransactionType    string            `json:"transaction_type"`
	GroupControlNumber string            `json:"group_control_number"`
	ASN                *ASNDocument      `json:"asn,omitempty"`
	ShipmentID         string            `json:"shipment_id"`
	ControlNumber      string            `json:"control_number"`
	DocumentNumber     string            `json:"document_number,omitempty"`
	DocumentDate       string            `json:"document_date,omitempty"`
	PurposeCode        string            `json:"purpose_code,omitempty"`
	TypeCode           string            `json:"type_code,omitempty"`
	PONumber           string            `json:"po_number,omitempty"`
	PODate             string            `json:"po_date,omitempty"`
	CarrierSCAC        string            `json:"carrier_scac,omitempty"`
	TotalAmount        float64           `json:"total_amount,omitempty"`
	References         map[string]string `json:"references,omitempty"`
	Dates              []DateRef         `json:"dates"`
	Parties            []Party           `json:"parties"`
	LineItems          []LineItem        `json:"line_items,omitempty"`
	Charges            []ChargeAllowance `json:"charges,omitempty"`
	ShipmentEvents     []ShipmentEvent   `json:"shipment_events,omitempty"`
	TotalLineItems     int               `json:"total_line_items,omitempty"`
}

func extractProductIDs(elems []string, startIdx int) map[string]string {
	ids := make(map[string]string)
	for i := startIdx; i+1 < len(elems); i += 2 {
		qual := safeElem(elems, i)
		val := safeElem(elems, i+1)
		if qual != "" && val != "" {
			ids[qual] = val
		}
	}
	return ids
}

// ParseTransaction dispatches a discrete ST..SE TransactionSet into a normalized ParsedTransaction.
func ParseTransaction(tx TransactionSet, delims Delimiters) (ParsedTransaction, error) {
	switch tx.TransactionType {
	case "850", "855", "810", "214":
		return parseSupplyChainTx(tx, delims), nil
	case "856":
		return Parse856(tx, delims), nil
	default:
		return ParsedTransaction{}, fmt.Errorf("unsupported transaction set '%s'; supported types: 850, 855, 810, 214", tx.TransactionType)
	}
}

func parseSupplyChainTx(tx TransactionSet, delims Delimiters) ParsedTransaction {
	doc := ParsedTransaction{
		TransactionType:    tx.TransactionType,
		ControlNumber:      tx.ControlNumber,
		References:         make(map[string]string),
		Dates:              make([]DateRef, 0),
		Parties:            make([]Party, 0),
		GroupControlNumber: tx.GroupCtrl,
	}

	var currentParty *Party
	var currentLine *LineItem
	var currentEvent *ShipmentEvent
	inLineLoop := false

	flushParty := func() {
		if currentParty != nil {
			doc.Parties = append(doc.Parties, *currentParty)
			currentParty = nil
		}
	}
	flushLine := func() {
		if currentLine != nil {
			doc.LineItems = append(doc.LineItems, *currentLine)
			currentLine = nil
		}
	}
	flushEvent := func() {
		if currentEvent != nil {
			doc.ShipmentEvents = append(doc.ShipmentEvents, *currentEvent)
			currentEvent = nil
		}
	}

	for _, seg := range tx.Segments {
		switch seg.ID {
		case "BEG": // 850 Purchase Order
			doc.PurposeCode = safeElem(seg.Elements, 0)
			doc.TypeCode = safeElem(seg.Elements, 1)
			doc.DocumentNumber = safeElem(seg.Elements, 2)
			doc.PONumber = doc.DocumentNumber
			doc.DocumentDate = safeElem(seg.Elements, 4)
			doc.PODate = doc.DocumentDate

		case "BAK": // 855 PO Acknowledgment
			doc.PurposeCode = safeElem(seg.Elements, 0)
			doc.TypeCode = safeElem(seg.Elements, 1)
			doc.PONumber = safeElem(seg.Elements, 2)
			doc.PODate = safeElem(seg.Elements, 3)
			doc.DocumentNumber = safeElem(seg.Elements, 7)
			if doc.DocumentNumber == "" {
				doc.DocumentNumber = doc.PONumber
			}
			doc.DocumentDate = safeElem(seg.Elements, 8)

		case "BIG": // 810 Invoice
			doc.DocumentDate = safeElem(seg.Elements, 0)
			doc.DocumentNumber = safeElem(seg.Elements, 1)
			doc.PODate = safeElem(seg.Elements, 2)
			doc.PONumber = safeElem(seg.Elements, 3)
			doc.TypeCode = safeElem(seg.Elements, 6)

		case "B10": // 214 Shipment Status
			doc.DocumentNumber = safeElem(seg.Elements, 0) // Carrier Reference / PRO
			doc.ShipmentID = safeElem(seg.Elements, 1)     // Shipment Identification Number (BOL/Shipment ID)
			doc.CarrierSCAC = safeElem(seg.Elements, 2)

		case "REF", "L11":
			val := safeElem(seg.Elements, 0)
			qual := safeElem(seg.Elements, 1)
			if seg.ID == "REF" {
				qual, val = val, qual
			}
			if qual != "" && val != "" {
				doc.References[qual] = val
			}

		case "DTM":
			d := DateRef{
				Qualifier: safeElem(seg.Elements, 0),
				Date:      safeElem(seg.Elements, 1),
				Time:      safeElem(seg.Elements, 2),
			}
			if d.Qualifier != "" {
				if inLineLoop && currentLine != nil {
					currentLine.Dates = append(currentLine.Dates, d)
				} else {
					doc.Dates = append(doc.Dates, d)
				}
			}

		case "N1":
			flushParty()
			flushLine()
			inLineLoop = false
			currentParty = &Party{
				EntityCode: safeElem(seg.Elements, 0),
				Name:       safeElem(seg.Elements, 1),
				IDQual:     safeElem(seg.Elements, 2),
				ID:         safeElem(seg.Elements, 3),
			}

		case "N3":
			if currentParty != nil {
				currentParty.AddressLine1 = safeElem(seg.Elements, 0)
				currentParty.AddressLine2 = safeElem(seg.Elements, 1)
			}

		case "N4":
			if currentParty != nil {
				currentParty.City = safeElem(seg.Elements, 0)
				currentParty.State = safeElem(seg.Elements, 1)
				currentParty.PostalCode = safeElem(seg.Elements, 2)
				currentParty.Country = safeElem(seg.Elements, 3)
			}

		case "PO1": // 850 & 855 Line Item
			flushParty()
			flushLine()
			inLineLoop = true
			qty, _ := strconv.ParseFloat(safeElem(seg.Elements, 1), 64)
			price, _ := strconv.ParseFloat(safeElem(seg.Elements, 3), 64)
			currentLine = &LineItem{
				LineNumber: safeElem(seg.Elements, 0),
				Quantity:   qty,
				UnitOfMeas: splitSubElem(safeElem(seg.Elements, 2), delims.SubElement),
				UnitPrice:  price,
				ProductIDs: extractProductIDs(seg.Elements, 5),
			}

		case "IT1": // 810 Invoice Line Item
			flushParty()
			flushLine()
			inLineLoop = true
			qty, _ := strconv.ParseFloat(safeElem(seg.Elements, 1), 64)
			price, _ := strconv.ParseFloat(safeElem(seg.Elements, 3), 64)
			currentLine = &LineItem{
				LineNumber: safeElem(seg.Elements, 0),
				Quantity:   qty,
				UnitOfMeas: splitSubElem(safeElem(seg.Elements, 2), delims.SubElement),
				UnitPrice:  price,
				ProductIDs: extractProductIDs(seg.Elements, 5),
			}

		case "PID":
			if currentLine != nil && safeElem(seg.Elements, 0) == "F" {
				currentLine.Description = safeElem(seg.Elements, 4)
			}

		case "ACK": // 855 Line Acknowledgment
			if currentLine != nil {
				currentLine.AckStatus = safeElem(seg.Elements, 0)
				currentLine.AckQuantity, _ = strconv.ParseFloat(safeElem(seg.Elements, 1), 64)
			}

		case "TDS": // 810 Total Monetary Value (implied 2 decimal places in X12)
			rawCents, err := strconv.ParseFloat(safeElem(seg.Elements, 0), 64)
			if err == nil {
				doc.TotalAmount = rawCents / 100.0
			}

		case "SAC": // Service, Promotion, Allowance, or Charge
			amtCents, _ := strconv.ParseFloat(safeElem(seg.Elements, 4), 64)
			doc.Charges = append(doc.Charges, ChargeAllowance{
				Indicator:   safeElem(seg.Elements, 0),
				Code:        safeElem(seg.Elements, 1),
				Amount:      amtCents / 100.0,
				Description: safeElem(seg.Elements, 14),
			})

		case "AT7": // 214 Shipment Status Details
			flushParty()
			flushEvent()
			currentEvent = &ShipmentEvent{
				StatusCode: safeElem(seg.Elements, 0),
				ReasonCode: safeElem(seg.Elements, 1),
				Date:       safeElem(seg.Elements, 4),
				Time:       safeElem(seg.Elements, 5),
				TimeZone:   safeElem(seg.Elements, 6),
			}

		case "MS1": // 214 Equipment, Shipment, or Real Property Location
			if currentEvent != nil {
				currentEvent.City = safeElem(seg.Elements, 0)
				currentEvent.State = safeElem(seg.Elements, 1)
				currentEvent.Country = safeElem(seg.Elements, 2)
			}

		case "MS2": // 214 Equipment or Container Owner and Type
			if currentEvent != nil {
				currentEvent.EquipmentInit = safeElem(seg.Elements, 0)
				currentEvent.EquipmentNum = safeElem(seg.Elements, 1)
			}

		case "CTT":
			flushParty()
			flushLine()
			inLineLoop = false
			doc.TotalLineItems, _ = strconv.Atoi(safeElem(seg.Elements, 0))

		case "SE":
			flushParty()
			flushLine()
			flushEvent()
		}
	}

	flushParty()
	flushLine()
	flushEvent()

	if doc.TotalLineItems == 0 && len(doc.LineItems) > 0 {
		doc.TotalLineItems = len(doc.LineItems)
	}

	return doc
}
