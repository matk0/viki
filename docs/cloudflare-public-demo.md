# Cloudflare options for the public Viki instance

Assessed 11 November 2026. The initial comparison considered an existing-host tunnel. The chosen implementation is the [Cloudflare-native public-demo adapter](../public-demo/README.md), with the shared reader, bundled descriptive sources, browser-tab-local chat, D1 quota counters, and Workers AI. The full Go/PostgreSQL/Hermes application remains a separate deployment option.

## What can be free

| Option | Verified terms | Fit for Viki |
| --- | --- | --- |
| Workers Static Assets or Pages | Static asset requests are free and unlimited. Dynamic Workers/Pages Functions on Free share a 100,000-request daily allowance; Workers Free permits 10 ms CPU per invocation. | Can host the React reader, but does not run Viki's existing backend stack. [Static assets](https://developers.cloudflare.com/workers/static-assets/billing-and-limitations/), [Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/), [Pages Functions pricing](https://developers.cloudflare.com/pages/functions/pricing/) |
| `workers.dev` or `pages.dev` address | Workers provides an account subdomain without onboarding a custom domain. Pages also supplies a project subdomain. | A public demo can avoid buying a domain. A free address does not supply a Go server or database. [Workers addresses](https://developers.cloudflare.com/workers/configuration/routing/workers-dev/), [Pages domains](https://developers.cloudflare.com/pages/configuration/custom-domains/) |
| Cloudflare Tunnel | Available on all plans; connects an existing server through outbound `cloudflared` connections. Publishing a stable application hostname directly requires a domain on Cloudflare. | Can expose a separate Viki instance on existing hardware. The hardware, storage, power, and availability remain the operator's responsibility. [Tunnel](https://developers.cloudflare.com/tunnel/), [prerequisites](https://developers.cloudflare.com/tunnel/get-started/) |
| Workers VPC | Available free on all Workers plans during open beta; standard Worker request/compute limits still apply. A VPC Service binds to one registered private host and port. | A possible `workers.dev` proxy to an existing private Viki server, without a purchased domain. This is a proposal, subject to an end-to-end streaming/cookie test and future beta pricing. [VPC overview](https://developers.cloudflare.com/workers-vpc/), [pricing](https://developers.cloudflare.com/workers-vpc/platform/pricing/), [API scope](https://developers.cloudflare.com/workers-vpc/api/) |
| Workers AI | Free allocation of 10,000 neurons per day. Free-plan operations fail when the allowance is exhausted; some models require paid billing. | Can provide quota-limited inference, subject to model compatibility and answer-quality validation. This is not unlimited free Q&A. [AI pricing](https://developers.cloudflare.com/workers-ai/platform/pricing/) |

Cloudflare Containers require Workers Paid, whose minimum is $5/month, with container resources metered separately. The Containers pricing table has no Free allowance. They can run existing container images, but are not a free drop-in host for this Compose stack. [Containers pricing](https://developers.cloudflare.com/containers/platform/pricing/), [Containers overview](https://developers.cloudflare.com/containers/)

## Chosen design

The public demo uses only Cloudflare infrastructure. It reuses the React reader and builds a catalogue from Viki's own Markdown and Gherkin, with a small Workers adapter for reading/search and stateless Q&A. It has no connection to the private Vozokany instance, internal importer/tool listeners, database, or home administration interfaces.

The Worker uses the account's existing `workers.dev` subdomain, so no domain purchase or tunnel is required. Cloudflare Static Assets serves the frontend; the Worker serves its API. Chat is kept in the individual browser tab, while D1 stores only daily request counters. One synchronous Workers AI call returns a structured answer and validated source citations. There is no public Hermes service or SSE dependency.

This adapter is limited to the public reader and Q&A. It does not port the full application's governance, review persistence, or Hermes runtime to D1. The original PostgreSQL implementation remains the full wiki's persistence layer. [D1](https://developers.cloudflare.com/d1/), [Viki persistence](../backend/internal/postgres/postgres.go), [initial database migration](../backend/internal/postgres/migrations/001_initial.sql)

For a tunnel-based deployment of the full application, a temporary Quick Tunnel is unsuitable: its address changes, it has no uptime guarantee, and it explicitly does not support the SSE used by Viki's Hermes integration. The chosen native adapter does not use a tunnel or SSE. [Quick Tunnel limits](https://developers.cloudflare.com/tunnel/get-started/quick-tunnels/#limitations), [full assistant stream](../backend/internal/app/assistant_http.go)

## Public Q&A boundary

The full application scopes conversations by organization and user but bootstraps one fixed account. Publishing shared credentials would share conversation access. The public adapter avoids that model: each tab retains its own history, and the server has no conversation-reading endpoint or conversation table. It rejects application writes, internal routes, Edit, and Developer capabilities. Tests cover two independent browser contexts. [Full conversation ownership](../backend/internal/postgres/assistant_conversations.go), [public adapter](../public-demo/src/app.mjs), [browser checks](../frontend/e2e/public-demo.spec.ts)

The adapter limits questions to five per visitor and ten per IP, with twenty model attempts globally per UTC day. It reserves all three counters atomically before initial inference; one permitted retry reserves another global attempt. Input, recent history, source excerpts, generated tokens, and attempt duration are bounded. Known transient errors and rejected structured answers can receive one retry; daily exhaustion, configuration errors, and refusals stop immediately. There are no billed fallbacks. Provider or application exhaustion pauses Q&A while reading/search remain available. D1 Free currently includes five million rows read and 100,000 rows written per day, 5 GB total storage, and ten databases; this app uses one small counter database. On Free, exhausted allowances cause errors rather than automatic upgrade. [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/), [budget and operations](../public-demo/README.md), [reliability policy](free-qa-reliability-research.md)

The selected model is gpt-oss-20b through a direct Workers AI binding; no Hermes endpoint is changed. The public reader is English-only, and its Q&A prompt asks the model to use the question's language. The full application's background translator remains a separate OpenAI Responses integration. [Model assessment](nvidia-public-qa.md), [translation provider](../backend/internal/localize/provider.go)

No paid-plan upgrade, domain purchase, home-network exposure, or NVIDIA trial API access is part of this implementation. Free allowances are account-wide; the application's counters are a conservative request cap rather than an account-wide billing meter.

## Verification notes

Cloudflare facts above were checked against its official `cloudflare/cloudflare-docs` repository's `production` branch on the assessment date. The published documentation URLs are linked beside the claims. The implementation's local Workers runtime, real model-call, browser, and release checks are documented with the release. The account's billing-subscription endpoint was unavailable to the existing OAuth scopes; no billing settings were changed.
