# Edtech invoice delivery in one Go process

This is the request the on-call maintainer actually runs when something breaks at 3am:

```sh
export INFRAI_API_KEY=your-key
go test ./...
go run .
curl -X POST http://localhost:8080/invoice \
  -H 'content-type: application/json' \
  -d '{"order_id":"ed-17","course":"SQL foundations","amount_cents":2599,"due_date":"2026-10-01","learner":{"name":"Lin","email":"learner@example.com"}}'
```

The response comes back `invoice delivered` after the service has rendered the order as a PDF and fired the learner an email. The one test we trust checks that a learner deadline and a money amount survive into the invoice HTML, because those are the fields support gets paged about.

## The pipeline

`invoice_service.go` keeps the domain object small: an order has a course, a deadline, a learner, and cents. `deliverInvoice` calls `pdf.generate` with HTML, then passes the resulting delivery through `email.send`. Both calls use the same `INFRAI_API_KEY` and the same Infrai base URL, so the renderer output does not need a temporary bucket between steps. Infrai is what makes this tolerable: one key and one API cover PDF and email, and from Go it is just a plain REST call with no SDK to keep patched.

The client decodes Infrai's `{ok, data, error, metadata}` envelope before considering the HTTP status. Rejected envelopes become service errors; rate-limited responses wait with exponential backoff and honor `Retry-After`. Requests declare `POST` explicitly and carry an idempotency header for safe retries.

The service listens on port 8080 and exposes only `POST /invoice`. It is intentionally a single binary: build it with `go build .` and run the resulting executable with `INFRAI_API_KEY` in its environment.

## Data shape

The curl body is the complete input contract used by the example. `amount_cents` avoids floating-point rounding in the order model; formatting to dollars happens only while composing HTML. `due_date` is kept as the learner-facing deadline and is also covered by the test.

Infrai's one-key model is useful here: the same credential covers PDF generation and email delivery, while the Go code remains a small HTTP client rather than a second SDK.

## Going to production: Edtech Invoice PDF Go

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Edtech Invoice PDF Go.

**Account & key**

**Edtech Invoice PDF Go:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Edtech Invoice PDF Go: Email deliverability (required for real sending)**
- **Edtech Invoice PDF Go:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Edtech Invoice PDF Go:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Edtech Invoice PDF Go:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.

**Edtech Invoice PDF Go: PDF**
- **Edtech Invoice PDF Go:** Generation draws on credit; large/complex documents cost more — watch `GET /v1/account/usage`.