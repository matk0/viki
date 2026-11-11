# Cloudflare options for the public Viki instance

Assessed 11 November 2026. These are hosting findings and proposals; no public instance or Cloudflare resources have been deployed.

## What can be free

| Option | Verified terms | Fit for Viki |
| --- | --- | --- |
| Workers Static Assets or Pages | Static asset requests are free and unlimited. Dynamic Workers/Pages Functions on Free share a 100,000-request daily allowance; Workers Free permits 10 ms CPU per invocation. | Can host the React reader, but does not run Viki's existing backend stack. [Static assets](https://developers.cloudflare.com/workers/static-assets/billing-and-limitations/), [Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/), [Pages Functions pricing](https://developers.cloudflare.com/pages/functions/pricing/) |
| `workers.dev` or `pages.dev` address | Workers provides an account subdomain without onboarding a custom domain. Pages also supplies a project subdomain. | A public demo can avoid buying a domain. A free address does not supply a Go server or database. [Workers addresses](https://developers.cloudflare.com/workers/configuration/routing/workers-dev/), [Pages domains](https://developers.cloudflare.com/pages/configuration/custom-domains/) |
| Cloudflare Tunnel | Available on all plans; connects an existing server through outbound `cloudflared` connections. Publishing a stable application hostname directly requires a domain on Cloudflare. | Can expose a separate Viki instance on existing hardware. The hardware, storage, power, and availability remain the operator's responsibility. [Tunnel](https://developers.cloudflare.com/tunnel/), [prerequisites](https://developers.cloudflare.com/tunnel/get-started/) |
| Workers VPC | Available free on all Workers plans during open beta; standard Worker request/compute limits still apply. A VPC Service binds to one registered private host and port. | A possible `workers.dev` proxy to an existing private Viki server, without a purchased domain. This is a proposal, subject to an end-to-end streaming/cookie test and future beta pricing. [VPC overview](https://developers.cloudflare.com/workers-vpc/), [pricing](https://developers.cloudflare.com/workers-vpc/platform/pricing/), [API scope](https://developers.cloudflare.com/workers-vpc/api/) |
| Workers AI | Free allocation of 10,000 neurons per day. Free-plan operations fail when the allowance is exhausted; some models require paid billing. | Can provide quota-limited inference, subject to model compatibility and answer-quality validation. This is not unlimited free Q&A. [AI pricing](https://developers.cloudflare.com/workers-ai/platform/pricing/) |

Cloudflare Containers require Workers Paid, whose minimum is $5/month, with container resources metered separately. The Containers pricing table has no Free allowance. They can run existing container images, but are not a free drop-in host for this Compose stack. [Containers pricing](https://developers.cloudflare.com/containers/platform/pricing/), [Containers overview](https://developers.cloudflare.com/containers/)

## Recommendation

Preserve the current Go/PostgreSQL/Hermes implementation. Create a separate public-demo instance containing only Viki's own specifications, with its own database, assistant state, and credentials. Route only that app's public HTTP listener through Cloudflare; keep the private Vozokany instance, internal importer/tool listeners, database, and home administration interfaces private.

For the fewest moving parts, use a named Tunnel and a subdomain on an existing Cloudflare-managed domain. If avoiding a custom domain matters, prototype a small Worker on `workers.dev` using a narrowly scoped VPC Service binding to the demo app. That route depends on a beta service and needs real SSE, cookies, reconnect, and citation-navigation checks before release. It still relies on the existing host; it is not entirely hosted by Cloudflare.

A Cloudflare-only Free demo would need a separate implementation proposal using Workers, a different datastore, and Workers AI. It would require replacing meaningful runtime/persistence behavior. For example, Cloudflare D1 uses SQLite semantics, while Viki currently uses PostgreSQL, extensions, and transactional repository code. That is a larger change than exposing a separate instance of the existing application. Its feasibility within all free-tier limits would need validation. [D1](https://developers.cloudflare.com/d1/), [Viki persistence](../backend/internal/postgres/postgres.go), [initial database migration](../backend/internal/postgres/migrations/001_initial.sql)

Do not use a temporary Quick Tunnel for this demo: its address changes, it has no uptime guarantee, and it explicitly does not support SSE. Viki streams assistant events over SSE. [Quick Tunnel limits](https://developers.cloudflare.com/tunnel/get-started/quick-tunnels/#limitations), [assistant stream](../backend/internal/app/assistant_http.go)

## Public Q&A work required

The current database scopes conversations by organization and user, but the app only bootstraps one fixed account and has no anonymous-visitor onboarding. Sharing that account would share conversation access. Add separate visitor identities and sessions, enforce ownership on conversation reads, events, messages, and stop actions, and regression-test two independent browser sessions. Browsing remains read-only; public visitors must never gain Edit or Developer capabilities. [Conversation ownership](../backend/internal/postgres/assistant_conversations.go), [bootstrap identity](../backend/internal/postgres/seed.go), [authentication](../backend/internal/app/auth.go)

Add per-visitor request limits, a global inference allowance, bounded input/output, and retention/cleanup for visitor conversations. Reaching the allowance should leave browsing available and clearly pause Q&A. These are implementation requirements for this public demo, not claims about existing behavior.

Workers AI exposes OpenAI-compatible Chat Completions for most text models. That makes it a candidate for Hermes's configurable OpenAI-compatible endpoint, but does not establish compatibility with Hermes's tool calls, streaming, or citation behavior. Validate those with the selected model before switching providers. Background translation is a separate path: its current provider uses a fixed OpenAI Responses endpoint, so changing the Hermes endpoint alone would not migrate translation. An English-only first release avoids that dependency. [Workers AI compatibility](https://developers.cloudflare.com/workers-ai/configuration/open-ai-compatibility/), [Hermes endpoint configuration](../hermes/render_config.py), [translation provider](../backend/internal/localize/provider.go)

Decisions still needed: existing-host versus Cloudflare-only operation; existing-domain versus `workers.dev` beta proxy; free inference quota versus the current provider with an explicitly chosen spending cap. No paid plan, new domain, or public ingress has been created.

## Verification notes

Cloudflare facts above were checked against its official `cloudflare/cloudflare-docs` repository's `production` branch on the assessment date. The published documentation URLs are linked beside the claims. Deployment behavior, existing Cloudflare account entitlements, and origin connectivity remain unverified.
