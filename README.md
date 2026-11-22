# EDI-Bridge

Go REST API and browser playground for parsing ANSI X12 Electronic Data Interchange (EDI) transactions into structured JSON and generating `997` Functional Acknowledgments[cite: 2, 5].

## Supported Transactions & Capabilities

* **X12 Parsers:** `850` (Purchase Order), `855` (PO Acknowledgment), `810` (Invoice), `214` (Transportation Carrier Shipment Status), and `856` (Advance Ship Notice / ASN with nested `HL` trees and `P`/`T` pack/tare hierarchies)[cite: 3, 5].
* **997 Functional Acknowledgments:** Generates `AK1`–`AK9` acknowledgments with custom delimiter support, `ISA15` (`P`/`T`) propagation, unique control numbers, and no embedded newline characters[cite: 3, 4, 5].
* **Envelope & Structural Validation:** Detects custom delimiters (`ISA`), validates `SE` / `GE` / `IEA` segment and transaction counts, and checks control-number alignment[cite: 4].
* **Dual Auth & Quota Middleware:** Supports `X-RapidAPI-Proxy-Secret` and SaaS `Authorization: Bearer <api_key>` tokens with `X-RateLimit-Limit` and `X-RateLimit-Remaining` headers and `429 Too Many Requests` enforcement when quotas are exhausted[cite: 2, 5].
* **Browser Playground:** Interactive UI in `web/index.html` with sample payloads, formatting/streaming toggles, file upload, and JSON / `997` downloads.

## Project Structure

* `cmd/api` — HTTP server entry point, router wiring, and integration tests
* `internal/handlers` — HTTP request handlers (such as `/v1/parse`)
* `internal/middleware` — Dual authentication, key store abstraction, and quota tracking
* `internal/x12` — Tokenizer, envelope validator, transaction/856 parsers, and `997` generator[cite: 3, 4, 5]
* `web` — Static web playground (`index.html`)
* `plans` — Architecture and implementation planning docs

## Running Locally & Tests

```bash
go mod tidy
go test -v -race ./...
go run ./cmd/api
```

## Environment Variables
* `PORT` — HTTP listen port (defaults to 8080)   
* `DEV_BYPASS_AUTH` — Set to true only for local development bypass; keep false in production   
* `RAPIDAPI_PROXY_SECRET` — Shared proxy secret for authenticating RapidAPI traffic

## Planned Cloud Stack
* `Backend API` - Go on Azure App Service
* `Frontend Playground` - Azure Static Web Apps (./web)
* `Persistent Quotas & Keys` - Azure Cosmos DB (planned)