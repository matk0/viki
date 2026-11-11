# Public Viki demo on Cloudflare

The public demo reuses Viki's React reader and presents the checked-in
[self-documenting catalogue](../specs/viki/README.md). It runs as one Worker with
Static Assets, a Workers AI binding, and a small D1 database for rate counters.
It does not run Go, PostgreSQL, or Hermes and has no connection to the private
Vozokany installation.

## Behavior and boundaries

- Anonymous readers can browse concepts, features, scenarios, and current
  source revisions, and search prose and steps.
- The catalogue is bundled at build time. Stable source IDs map to deterministic
  page IDs; changed content changes the revision ID. There is no retained
  history of earlier deployments. These descriptive scenarios have no Cucumber
  bindings and are not presented as verified execution evidence.
  The public reader loads once rather than continuously polling the bundled
  catalogue, avoiding unnecessary dynamic requests while a tab stays open.
- Questions use synchronous gpt-oss-20b inference through Cloudflare Workers
  AI. Retrieval selects up to six public source excerpts. Responses use strict
  JSON with citation IDs enumerated from those sources; output is still validated
  server-side. Known capacity/timeouts and rejected JSON, citations, or token
  truncation can receive one retry with a short jittered delay. Each attempt has
  a native 15-second cancellation signal. Daily provider exhaustion, configuration
  errors, unknown upstream errors, and content filtering are not retried.
  No model tools or application writes are available.
- Each tab keeps its own chat in React state. Only the last four bounded
  messages are sent with a follow-up question. Reloading clears the history.
  No server conversation table, shared login, or conversation-reading endpoint
  exists.
- D1 stores daily quota buckets, including daily hashes of visitor and IP values.
  It stores no questions or answers. A scheduled job deletes buckets older than
  two UTC dates.

The shared frontend's normal build still uses the full application's login,
Hermes assistant, review interface, and language preferences. Only
`VITE_PUBLIC_DEMO=true` selects the public presentation.

## Free-tier budget

Keep the account on Workers Free; these commands do not upgrade its plan or add
a paid provider. Cloudflare's free allowances are account-wide, so other
applications also consume them. Provider limits can pause Q&A even before this
app reaches its own limit.

Initial application ceilings:

| Limit | Value |
| --- | --- |
| Global model attempts, including retries | 20 per UTC day |
| Visitor questions | 5 per UTC day |
| IP questions | 10 per UTC day |
| Question | 1,000 characters |
| Prior messages | 4, at most 800 characters each |
| Serialized model input | 14,000 UTF-8 bytes |
| Generated tokens, including reasoning | 1,536 |
| Attempts per question | At most 2, each with a 15-second timeout |
| Worker CPU | 10 ms per request, enforced by Workers Free |

One atomic D1 statement checks and increments the visitor/IP question counters
and global model-attempt counter before inference. A retry atomically reserves
another global attempt; it does not count as another visitor/IP question.
Failed and aborted calls consume reservations too. Retries reduce the number
of questions that fit into the unchanged global allowance. There are no
model/provider fallbacks. Exhaustion leaves browsing and search available.

Cloudflare Free currently includes 100,000 dynamic requests per day and 10,000
Workers AI neurons per day. Static asset requests are free and unlimited.
D1 has its own free storage/read/write allowances; this app writes only a few
counter rows per question. See the dated [hosting assessment](../docs/cloudflare-public-demo.md)
and [model assessment](../docs/nvidia-public-qa.md). The app's request cap is
conservative, not a precise meter of the account's remaining neuron allowance.

Do not configure `limits.cpu_ms` for the Free plan. Cloudflare rejects that
setting even when it equals the automatic 10 ms Free limit.

The selected open model also appears in NVIDIA's catalogue. Inference here uses
Cloudflare's hosted model, not NVIDIA's internal-evaluation trial API. No NVIDIA
API key is required.

## Diagnose failures

Failed inference attempts emit an allowlisted failure kind, numeric provider
code when available, attempt number, and random request reference. A final Q&A
error shows that reference to the visitor. The application emits no questions,
history, generated text, raw provider errors, headers, or cookies to logs.

Workers Logs persists these custom events within its Free allowance. Automatic
invocation logs and traces are disabled and URL query strings are redacted;
Cloudflare still adds platform context to custom events. The initial generic
503 cannot be attributed precisely because it predates these diagnostics.
See the [investigation and provider assessment](../docs/free-qa-reliability-research.md)
for the evidence, error-code policy, logging allowances, and API-key decision.

## Local checks

Install dependencies from the repository root:

```sh
npm --prefix frontend ci
npm --prefix public-demo ci
```

Run the API/SQLite checks and the public build:

```sh
npm --prefix public-demo test
npm --prefix public-demo run build
```

The exporter uses the official Gherkin parser, rejects unsupported constructs,
and resolves source identities and references. It writes the ignored
`src/catalogue.json`; do not hand-edit or commit that generated file.

Run browser checks using installed Google Chrome:

```sh
npm --prefix public-demo run test:e2e
```

These Chrome tests run the actual local Workers runtime and mock only model
answers. They check reading, search, citation links, two independent visitor
contexts, mobile layout, and quota errors. Manual questions in `wrangler dev`
use the real remote AI binding and consume the account's inference allowance.

## Release

Use existing Cloudflare authorization; do not commit credentials. The tracked
Wrangler config identifies this demo's database. A fork needs its own database
and Worker name.

From `public-demo/`, after the repository's required checks and main CI pass:

```sh
npx wrangler whoami
npm run migrate:remote
npm run deploy
```

The deployment builds the shared frontend with the explicit public flag and
copies the public security headers. Wrangler reports the actual `workers.dev`
URL and deployment version. Check `/readyz`, the catalogue, a real question,
and its revision citations after release. A build alone does not verify a
deployed model response.

Future releases are manual. No Cloudflare credentials are stored in GitHub.
For an application rollback, use `npx wrangler rollback` after inspecting
`npx wrangler deployments list`. The quota schema has no application content.
