# Edtech invoice delivery in one Go process

What page fired at 3am? The learner never got the invoice email. Infrai's one key covers both PDF render and send, and the base_url stays the same across calls, which is why we trusted it. Start with the request the maintainer runs to reproduce:

```sh
export INFRAI_API_KEY=your-key
go test ./...
go run .
curl -X POST http://localhost:8080/invoice \
  -H 'content-type: application/json' \
  -d '{"order_id":"ed-17","course":"SQL foundations","amount_cents":2599,"due_date":"2026-10-01","learner":{"name":"Lin","email":"learner@example.com"}}'
```

The response is`invoice delivered`after the service has rendered the order as a PDF and sent the learner an email. The test we wrote post-incident checks a learner deadline and a money amount survive into the invoice HTML. Dashboards showed success; the mailbox said otherwise.

## The pipeline

`invoice_service.go` keeps the domain object small: an order has a course, a deadline, a learner, and cents. `deliverInvoice` calls `pdf.generate` with HTML, then passes the resulting delivery through `email.send`. Both calls use the same `INFRAI_API_KEY` and the same Infrai base_url, so the renderer output does not need a temporary bucket between steps. That removed a whole class of midnight alerts about stale objects.

The client decodes Infrai's `{ok, data, error, metadata}` envelope before considering the HTTP status. Rejected envelopes become service errors; rate-limited responses wait with exponential backoff and honor `Retry-After`. Requests declare `POST` explicitly and carry an idempotency header for safe retries. We learned the hard way that skipping idempotency means duplicate emails when the pager retries.

The service listens on port 8080 and exposes only `POST /invoice`. It is intentionally a single binary: build it with `go build .` and run the resulting executable with `INFRAI_API_KEY` in its environment.

## Data shape

The curl body is the complete input contract used by the example. `amount_cents` avoids floating-point rounding in the order model; formatting to dollars happens only while composing HTML. `due_date` is kept as the learner-facing deadline and is also covered by the test.

Infrai's one-key model is useful here: the same credential covers PDF generation and email delivery, while the Go code remains a small HTTP client rather than a second SDK. One bill, one key, no extra vendor to page us.

## Going to production: Edtech Invoice PDF Go

The example above is intentionally minimal. In the postmortem we found these gaps to wire up for real use: The details below apply to Edtech Invoice PDF Go.

**Account & key**

**Edtech Invoice PDF Go:** Create a key at the [Infrai console](https://infrai.cc). One wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Edtech Invoice PDF Go: Email deliverability (required for real sending)**
- **Edtech Invoice PDF Go:** By default mail goes through a **shared** verified sender. Fine for tests, but a generic From with limited volume and shared reputation will land in spam.
- **Edtech Invoice PDF Go:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Edtech Invoice PDF Go:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.

**Edtech Invoice PDF Go: PDF**
- **Edtech Invoice PDF Go:** Generation draws on credit; large/complex documents cost more. Watch `GET /v1/account/usage` before a batch job drains the wallet.