package x12

import (
	"strconv"
)

type ASNItem struct {
	LineID       string            `json:"line_id,omitempty"`
	Quantity     float64           `json:"quantity"`
	UnitOfMeas   string            `json:"uom,omitempty"`
	ProductIDs   map[string]string `json:"product_ids,omitempty"`
	Description  string            `json:"description,omitempty"`
	LotNumber    string            `json:"lot_number,omitempty"`
	SerialNumber string            `json:"serial_number,omitempty"`
	Dates        []DateRef         `json:"dates,omitempty"`
}

type ASNPack struct {
	PackID       string            `json:"pack_id"`
	LevelCode    string            `json:"level_code,omitempty"` // "P" (Pack) or "T" (Tare)
	Packaging    string            `json:"packaging_code,omitempty"`
	Quantity     float64           `json:"quantity,omitempty"`
	UnitOfMeas   string            `json:"uom,omitempty"`
	TrackingNum  string            `json:"tracking_number,omitempty"`
	MarksNumbers []string          `json:"marks_and_numbers,omitempty"`
	References   map[string]string `json:"references,omitempty"`
	SubPacks     []ASNPack         `json:"sub_packs,omitempty"`
	Items        []ASNItem         `json:"items"`
}

type ASNOrder struct {
	OrderID     string            `json:"order_id"`
	PONumber    string            `json:"po_number,omitempty"`
	PODate      string            `json:"po_date,omitempty"`
	References  map[string]string `json:"references,omitempty"`
	Parties     []Party           `json:"parties,omitempty"`
	Dates       []DateRef         `json:"dates,omitempty"`
	Packs       []ASNPack         `json:"packs"`
	DirectItems []ASNItem         `json:"direct_items,omitempty"`
}

type ASNShipment struct {
	ShipmentID    string            `json:"shipment_id"`
	ShipmentDate  string            `json:"shipment_date,omitempty"`
	ShipmentTime  string            `json:"shipment_time,omitempty"`
	CarrierSCAC   string            `json:"carrier_scac,omitempty"`
	Routing       string            `json:"routing,omitempty"`
	BOLNumber     string            `json:"bol_number,omitempty"`
	PackagingCode string            `json:"packaging_code,omitempty"`
	GrossWeight   float64           `json:"gross_weight,omitempty"`
	WeightUOM     string            `json:"weight_uom,omitempty"`
	TotalPackages int               `json:"total_packages,omitempty"`
	References    map[string]string `json:"references,omitempty"`
	Parties       []Party           `json:"parties,omitempty"`
	Dates         []DateRef         `json:"dates,omitempty"`
	Orders        []ASNOrder        `json:"orders"`
}

type ASNDocument struct {
	PurposeCode  string        `json:"purpose_code,omitempty"`
	ShipmentID   string        `json:"shipment_id,omitempty"`
	ShipmentDate string        `json:"shipment_date,omitempty"`
	ShipmentTime string        `json:"shipment_time,omitempty"`
	Shipments    []ASNShipment `json:"shipments"`
}

type hlNode struct {
	ID       string
	ParentID string
	Level    string
	Segments []Segment
}

// Parse856 builds a nested Shipment / Order / Pack / Tare / Item hierarchy from an 856 transaction set,
// supporting SOI, SOPI, SOTPI, and STOPI HL loop structures.
func Parse856(tx TransactionSet, delims Delimiters) ParsedTransaction {
	doc := ParsedTransaction{
		TransactionType:    tx.TransactionType,
		GroupControlNumber: tx.GroupCtrl,
		ControlNumber:      tx.ControlNumber,
		References:         make(map[string]string),
		Dates:              make([]DateRef, 0),
		Parties:            make([]Party, 0),
		ASN: &ASNDocument{
			Shipments: make([]ASNShipment, 0),
		},
	}

	var nodes []*hlNode
	var curNode *hlNode
	var headerSegs []Segment
	var shipDate, shipTime string

	for _, seg := range tx.Segments {
		switch seg.ID {
		case "ST", "SE":
			continue
		case "BSN":
			doc.PurposeCode = safeElem(seg.Elements, 0)
			doc.DocumentNumber = safeElem(seg.Elements, 1)
			doc.ShipmentID = doc.DocumentNumber
			shipDate = safeElem(seg.Elements, 2)
			shipTime = safeElem(seg.Elements, 3)
			doc.DocumentDate = shipDate
			doc.ASN.PurposeCode = doc.PurposeCode
			doc.ASN.ShipmentID = doc.ShipmentID
			doc.ASN.ShipmentDate = shipDate
			doc.ASN.ShipmentTime = shipTime
		case "HL":
			curNode = &hlNode{
				ID:       safeElem(seg.Elements, 0),
				ParentID: safeElem(seg.Elements, 1),
				Level:    safeElem(seg.Elements, 2),
			}
			nodes = append(nodes, curNode)
		default:
			if curNode != nil {
				curNode.Segments = append(curNode.Segments, seg)
			} else {
				headerSegs = append(headerSegs, seg)
			}
		}
	}

	applyHeaderOrLevelSegments(headerSegs, delims, &doc.References, &doc.Dates, &doc.Parties, nil, nil, nil, nil, "")

	nodesByID := make(map[string]*hlNode, len(nodes))
	for _, n := range nodes {
		nodesByID[n.ID] = n
	}

	findAncestorID := func(startParentID string, targets ...string) string {
		visited := make(map[string]bool)
		curID := startParentID
		for curID != "" && !visited[curID] {
			visited[curID] = true
			pNode, ok := nodesByID[curID]
			if !ok {
				break
			}
			for _, t := range targets {
				if pNode.Level == t {
					return pNode.ID
				}
			}
			curID = pNode.ParentID
		}
		return ""
	}

	getShipmentByHL := func(hlID string) *ASNShipment {
		for i := range doc.ASN.Shipments {
			if doc.ASN.Shipments[i].ShipmentID == hlID {
				return &doc.ASN.Shipments[i]
			}
		}
		if len(doc.ASN.Shipments) > 0 {
			return &doc.ASN.Shipments[len(doc.ASN.Shipments)-1]
		}
		return nil
	}

	var findOrderInPacks func(packs []ASNPack, orderHLID string) *ASNOrder
	findOrderInPacks = func(packs []ASNPack, orderHLID string) *ASNOrder {
		for i := range packs {
			if o := findOrderInPacks(packs[i].SubPacks, orderHLID); o != nil {
				return o
			}
		}
		return nil
	}

	findOrderInSlice := func(orders []ASNOrder, orderHLID string) *ASNOrder {
		for i := range orders {
			if orders[i].OrderID == orderHLID {
				return &orders[i]
			}
			if found := findOrderInPacks(orders[i].Packs, orderHLID); found != nil {
				return found
			}
		}
		return nil
	}
	getOrderByHL := func(orderHLID string) *ASNOrder {
		for i := range doc.ASN.Shipments {
			if found := findOrderInSlice(doc.ASN.Shipments[i].Orders, orderHLID); found != nil {
				return found
			}
		}
		return nil
	}

	var findPackInSlice func(packs []ASNPack, packHLID string) *ASNPack
	findPackInSlice = func(packs []ASNPack, packHLID string) *ASNPack {
		for i := range packs {
			if packs[i].PackID == packHLID {
				return &packs[i]
			}
			if nested := findPackInSlice(packs[i].SubPacks, packHLID); nested != nil {
				return nested
			}
		}
		return nil
	}

	getPackByHL := func(packHLID string) *ASNPack {
		for i := range doc.ASN.Shipments {
			for j := range doc.ASN.Shipments[i].Orders {
				if found := findPackInSlice(doc.ASN.Shipments[i].Orders[j].Packs, packHLID); found != nil {
					return found
				}
			}
		}
		return nil
	}

	ensureDefaultShipment := func() *ASNShipment {
		if len(doc.ASN.Shipments) == 0 {
			doc.ASN.Shipments = append(doc.ASN.Shipments, ASNShipment{
				ShipmentID:   doc.ShipmentID,
				ShipmentDate: shipDate,
				ShipmentTime: shipTime,
				References:   make(map[string]string),
				Parties:      make([]Party, 0),
				Dates:        make([]DateRef, 0),
				Orders:       make([]ASNOrder, 0),
			})
		}
		return &doc.ASN.Shipments[len(doc.ASN.Shipments)-1]
	}

	ensureDefaultOrder := func(ship *ASNShipment) *ASNOrder {
		if len(ship.Orders) == 0 {
			ship.Orders = append(ship.Orders, ASNOrder{
				OrderID:    "DEFAULT",
				References: make(map[string]string),
				Parties:    make([]Party, 0),
				Dates:      make([]DateRef, 0),
				Packs:      make([]ASNPack, 0),
			})
		}
		return &ship.Orders[len(ship.Orders)-1]
	}

	for _, n := range nodes {
		switch n.Level {
		case "S":
			shipID := doc.ShipmentID
			if shipID == "" {
				shipID = n.ID
			}
			ship := ASNShipment{
				ShipmentID:   shipID,
				ShipmentDate: shipDate,
				ShipmentTime: shipTime,
				References:   make(map[string]string),
				Parties:      make([]Party, 0),
				Dates:        make([]DateRef, 0),
				Orders:       make([]ASNOrder, 0),
			}
			applyHeaderOrLevelSegments(n.Segments, delims, &ship.References, &ship.Dates, &ship.Parties, &ship, nil, nil, nil, "")
			if ship.BOLNumber != "" && doc.DocumentNumber == "" {
				doc.DocumentNumber = ship.BOLNumber
			}
			doc.ASN.Shipments = append(doc.ASN.Shipments, ship)

		case "O":
			ord := ASNOrder{
				OrderID:    n.ID,
				References: make(map[string]string),
				Parties:    make([]Party, 0),
				Dates:      make([]DateRef, 0),
				Packs:      make([]ASNPack, 0),
			}
			applyHeaderOrLevelSegments(n.Segments, delims, &ord.References, &ord.Dates, &ord.Parties, nil, &ord, nil, nil, "")
			if ord.PONumber != "" && doc.PONumber == "" {
				doc.PONumber = ord.PONumber
			}

			ship := ensureDefaultShipment()
			if shipHL := findAncestorID(n.ParentID, "S"); shipHL != "" {
				if s := getShipmentByHL(shipHL); s != nil {
					ship = s
				}
			}
			ship.Orders = append(ship.Orders, ord)

		case "P", "T":
			pack := ASNPack{
				PackID:     n.ID,
				LevelCode:  n.Level,
				References: make(map[string]string),
				SubPacks:   make([]ASNPack, 0),
				Items:      make([]ASNItem, 0),
			}
			applyHeaderOrLevelSegments(n.Segments, delims, &pack.References, nil, nil, nil, nil, &pack, nil, "")

			if parentPackHL := findAncestorID(n.ParentID, "P", "T"); parentPackHL != "" {
				if p := getPackByHL(parentPackHL); p != nil {
					p.SubPacks = append(p.SubPacks, pack)
					break
				}
			}

			var targetOrd *ASNOrder
			if ordHL := findAncestorID(n.ParentID, "O"); ordHL != "" {
				targetOrd = getOrderByHL(ordHL)
			}
			if targetOrd == nil {
				ship := ensureDefaultShipment()
				if shipHL := findAncestorID(n.ParentID, "S"); shipHL != "" {
					if s := getShipmentByHL(shipHL); s != nil {
						ship = s
					}
				}
				targetOrd = ensureDefaultOrder(ship)
			}
			targetOrd.Packs = append(targetOrd.Packs, pack)

		case "I":
			item := ASNItem{
				LineID:     n.ID,
				ProductIDs: make(map[string]string),
				Dates:      make([]DateRef, 0),
			}
			applyHeaderOrLevelSegments(n.Segments, delims, nil, &item.Dates, nil, nil, nil, nil, &item, delims.SubElement)

			doc.LineItems = append(doc.LineItems, LineItem{
				LineNumber:  item.LineID,
				Quantity:    item.Quantity,
				UnitOfMeas:  item.UnitOfMeas,
				ProductIDs:  item.ProductIDs,
				Description: item.Description,
				Dates:       item.Dates,
			})

			if packHL := findAncestorID(n.ParentID, "P", "T"); packHL != "" {
				if p := getPackByHL(packHL); p != nil {
					p.Items = append(p.Items, item)
					break
				}
			}
			if ordHL := findAncestorID(n.ParentID, "O"); ordHL != "" {
				if o := getOrderByHL(ordHL); o != nil {
					o.DirectItems = append(o.DirectItems, item)
					break
				}
			}
			ship := ensureDefaultShipment()
			ord := ensureDefaultOrder(ship)
			ord.DirectItems = append(ord.DirectItems, item)
		}
	}

	doc.TotalLineItems = len(doc.LineItems)
	return doc
}

func applyHeaderOrLevelSegments(
	segs []Segment,
	delims Delimiters,
	refs *map[string]string,
	dates *[]DateRef,
	parties *[]Party,
	ship *ASNShipment,
	ord *ASNOrder,
	pack *ASNPack,
	item *ASNItem,
	subElemSep string,
) {
	var curParty *Party

	flushParty := func() {
		if curParty != nil && parties != nil {
			*parties = append(*parties, *curParty)
			curParty = nil
		}
	}

	for _, seg := range segs {
		switch seg.ID {
		case "PRF":
			if ord != nil {
				ord.PONumber = safeElem(seg.Elements, 0)
				ord.PODate = safeElem(seg.Elements, 3)
			}
		case "TD1":
			if ship != nil {
				ship.PackagingCode = safeElem(seg.Elements, 0)
				ship.TotalPackages, _ = strconv.Atoi(safeElem(seg.Elements, 1))
				ship.GrossWeight, _ = strconv.ParseFloat(safeElem(seg.Elements, 6), 64)
				ship.WeightUOM = splitSubElem(safeElem(seg.Elements, 7), delims.SubElement)
			}
		case "TD5":
			if ship != nil {
				ship.CarrierSCAC = safeElem(seg.Elements, 2)
				ship.Routing = safeElem(seg.Elements, 4)
			}
		case "REF":
			qual := safeElem(seg.Elements, 0)
			val := safeElem(seg.Elements, 1)
			if qual != "" && val != "" {
				if refs != nil {
					(*refs)[qual] = val
				}
				if ship != nil && (qual == "BM" || qual == "CN") {
					ship.BOLNumber = val
				}
				if pack != nil && (qual == "LS" || qual == "CR" || qual == "2I") {
					pack.TrackingNum = val
				}
				if item != nil && qual == "LT" {
					item.LotNumber = val
				}
				if item != nil && qual == "SE" {
					item.SerialNumber = val
				}
			}
		case "MAN":
			if pack != nil {
				mark := safeElem(seg.Elements, 1)
				if mark != "" {
					pack.MarksNumbers = append(pack.MarksNumbers, mark)
				}
			}
		case "DTM":
			if dates != nil {
				d := DateRef{
					Qualifier: safeElem(seg.Elements, 0),
					Date:      safeElem(seg.Elements, 1),
					Time:      safeElem(seg.Elements, 2),
				}
				if d.Qualifier != "" {
					*dates = append(*dates, d)
				}
			}
		case "N1":
			flushParty()
			if parties != nil {
				curParty = &Party{
					EntityCode: safeElem(seg.Elements, 0),
					Name:       safeElem(seg.Elements, 1),
					IDQual:     safeElem(seg.Elements, 2),
					ID:         safeElem(seg.Elements, 3),
				}
			}
		case "N3":
			if curParty != nil {
				curParty.AddressLine1 = safeElem(seg.Elements, 0)
				curParty.AddressLine2 = safeElem(seg.Elements, 1)
			}
		case "N4":
			if curParty != nil {
				curParty.City = safeElem(seg.Elements, 0)
				curParty.State = safeElem(seg.Elements, 1)
				curParty.PostalCode = safeElem(seg.Elements, 2)
				curParty.Country = safeElem(seg.Elements, 3)
			}
		case "LIN":
			if item != nil {
				if safeElem(seg.Elements, 0) != "" {
					item.LineID = safeElem(seg.Elements, 0)
				}
				item.ProductIDs = extractProductIDs(seg.Elements, 1)
			}
		case "SN1":
			if item != nil {
				if safeElem(seg.Elements, 0) != "" {
					item.LineID = safeElem(seg.Elements, 0)
				}
				item.Quantity, _ = strconv.ParseFloat(safeElem(seg.Elements, 1), 64)
				item.UnitOfMeas = splitSubElem(safeElem(seg.Elements, 2), subElemSep)
			}
		case "PID":
			if item != nil && safeElem(seg.Elements, 0) == "F" {
				item.Description = safeElem(seg.Elements, 4)
			}
		}
	}
	flushParty()
}
