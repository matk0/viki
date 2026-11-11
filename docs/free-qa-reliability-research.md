# Free public Q&A: reliability choices

Assessed 11 November 2026. This note covers provider contracts and implementation
choices. This research made no inference requests, account changes, API keys, or
provider agreements. Actual reproduction and release evidence belongs to the
implementation investigation.

## Recommendation

Keep the authenticated Workers AI binding and `@cf/openai/gpt-oss-20b`. Add a
bounded, classified retry and useful, content-free diagnostics before changing
providers. An API key is not evidence of better inference availability.

Cloudflare documents the binding as the native way for a Worker to invoke
Workers AI. Its REST interface requires an account ID and API token. Neither
document establishes a separate reliability tier for REST credentials. The
conclusion that a new Cloudflare API key would not itself fix generation or
validation failures is an engineering inference; no provider comparison or
availability guarantee was measured. [Binding](https://developers.cloudflare.com/workers-ai/configuration/bindings/),
[REST authentication](https://developers.cloudflare.com/workers-ai/get-started/rest-api/)

The earlier same-day [NVIDIA assessment](nvidia-public-qa.md#why-the-inference-provider-matters)
found that NVIDIA's hosted trial terms do not establish permission for this
public Q&A service. A key does not change those terms. This follow-up did not
reinspect the NVIDIA PDF or establish a different subscription. Keep the existing
Cloudflare route unless a different endpoint's public-use rights, hard free
limits, and actual reliability are established. [Catalogue-linked trial terms](https://assets.ngc.nvidia.com/products/api-catalog/legal/NVIDIA%20API%20Trial%20Terms%20of%20Service.pdf)

## Distinguish failures before retrying

Cloudflare's current error table separates these cases:

| Internal code | HTTP status | Meaning | Suggested application handling |
| --- | --- | --- | --- |
| `3040` | `429` | Temporary model capacity shortage | One bounded retry |
| `3007` | `408` | Inference request timed out | One bounded retry |
| `3008` | `408` | Inference request aborted | One bounded retry if it was an upstream/attempt timeout, not an explicit visitor cancellation |
| `3036` | `429` | Daily free neuron allowance exhausted | Stop; no retry |
| Invalid input/model, permissions, account block, paid-plan requirement | `400` / `403` / `404` / `410` | Configuration or access failure | Stop; no retry or paid fallback |

The same HTTP status can mean temporary capacity or daily exhaustion. A blanket
retry of every `429` would be wrong. Unknown errors should remain visible and
receive a safe diagnostic classification; do not assume they are transient.
[Error reference](https://developers.cloudflare.com/workers-ai/platform/errors/)

The runtime creates `InferenceUpstreamError` instances. For structured provider
errors it prefixes the message with a numeric internal code, but the error name
can be supplied by the upstream service. Extract only an anchored, allowlisted
numeric code for classification; never log the complete provider message,
questions, history, output, or credentials. The binding's mutable
`lastRequestHttpStatusCode` / `lastRequestInternalStatusCode` fields are shared
state and are not a reliable per-request correlation mechanism under concurrent
calls. [Runtime implementation](https://github.com/cloudflare/workerd/blob/main/src/cloudflare/internal/ai-api.ts)

The original application's generic `503` and `error.name` alone do not establish
which of inference, completion, JSON parsing, or citation validation failed.
Preserve that historical uncertainty; a simulated failure is a regression test,
not proof of the original cause.

## Timeout and structured output

Current runtime types and implementation support an abort signal in the **third**
argument: `AI.run(model, input, { signal })`. The runtime forwards it to its
internal fetch. Give each attempt a finite timeout; a plain `Promise.race`
without cancellation can leave inference running. Cancellation does not prove
that no generation was billed, so keep the attempt reservation consumed.
[Types](https://github.com/cloudflare/workerd/blob/main/types/defines/ai.d.ts),
[Signal forwarding](https://github.com/cloudflare/workerd/blob/main/src/cloudflare/internal/ai-api.ts)

GPT-OSS supports low, medium, and high reasoning. Reasoning cannot be disabled
for this model. Keep `reasoning.effort: 'low'` and one consistent Responses
interface. Completion output can report `incomplete_details.reason` as
`max_output_tokens` or `content_filter`; do not conflate those conditions.
[Model metadata](https://github.com/cloudflare/cloudflare-docs/blob/production/src/content/workers-ai-models/gpt-oss-20b.json),
[Response and model types](https://github.com/cloudflare/workerd/blob/main/types/defines/ai.d.ts)

Cloudflare explicitly does not guarantee that schema requests always succeed.
Use a schema whose citation items enumerate the selected source IDs and still
validate output server-side. One fresh attempt for invalid/truncated structured
output is a reasonable application policy, not a documented recovery guarantee.
Do not retry refusal/content filtering or silently accept uncited free-form text.
[JSON Mode limitations](https://developers.cloudflare.com/workers-ai/features/json-mode/)

## Keep retries inside the existing free budget

Workers Free supplies 10,000 neurons per UTC day and errors after exhaustion;
excess usage requires an explicit Paid-plan change. GPT-OSS-20B is currently
outside the pricing page's paid-only model list. Its rates are 18,182 neurons per
million input tokens and 27,273 per million output tokens. The allowance is
account-wide, including local Wrangler inference. [Pricing](https://developers.cloudflare.com/workers-ai/platform/pricing/),
[Limits](https://developers.cloudflare.com/workers-ai/platform/limits/)

The simplest ceiling is **20 model attempts per UTC day, including retries**,
with the existing visitor/IP submission ceilings. Reserve the initial attempt
atomically with the visitor/IP counters; a retry must atomically reserve one
more global attempt before invoking the model. Retry at most once. Failed and
aborted attempts count. Document attempts consistently rather than promising 20
successful questions.

For scale only, a deliberately pessimistic 14,000 input-token call plus 1,536
output tokens consumes approximately 296.4 neurons: 20 such calls are about
5,929 neurons, whereas 40 calls would be about 11,858. The application's 14,000
UTF-8-byte input limit is **not** an account-wide token meter and this arithmetic
does not formally account for every provider template/schema token. Preserve
the Free plan's hard stop and headroom for other account usage. Do not double
the possible model-call count merely because each visitor question may retry.

## Free diagnostics without invocation logging

Current Workers Logs includes 200,000 log events per day with three-day retention
on Workers Free. On **1 December 2026**, the announced Cloudflare Observability
pricing supplies Free accounts with 0.5 GB of ingestion per day and seven-day
retention. Ingestion stops at the Free daily cap and resumes at 00:00 UTC; extra
paid ingestion is unavailable on Free. Allowances remain account-wide.
[Current pricing source](https://github.com/cloudflare/cloudflare-docs/blob/production/src/content/partials/workers/workers_logs_pricing.mdx),
[Announced Observability pricing](https://developers.cloudflare.com/observability/pricing/),
[Workers Logs](https://developers.cloudflare.com/workers/observability/logs/workers-logs/)

Wrangler supports persisted custom logs with `observability.logs.enabled: true`,
`head_sampling_rate: 1`, and `logs.invocation_logs: false`. This disables the
automatic invocation event containing request/response details while retaining
explicit console events. Set `redact_query_string: true` and disable traces as
well. These fields are present in the project's pinned Wrangler 4.158.0 schema.
[Configuration](https://developers.cloudflare.com/workers/wrangler/configuration/#observability),
[Invocation log switch](https://developers.cloudflare.com/workers/observability/logs/workers-logs/#invocation-logs)

Emit only allowlisted failure kinds, numeric provider codes, random request IDs,
and attempt counts. A design with at most one event per model attempt and one
final-failure event per submitted question would produce at most 40 **Q&A**
diagnostic events from the 20-attempt ceiling. Other application errors and
account Workers consume the same logging allowance. Do not emit raw exceptions,
prompts, answers, cookies, or headers. Custom logs are context-enriched by the
platform, so disabling invocation events is not a promise that the provider
stores literally only the custom object's fields.

## Source inspection

Context7 supplied current Cloudflare documentation. Exact error contracts,
pricing, schema limitations, and timeout behavior were verified against the
official `cloudflare/cloudflare-docs` and `cloudflare/workerd` repositories
through the GitHub connector. Google Chrome tab creation timed out twice, so
this follow-up made no successful browser inspection or alternative-provider
signup. No claim of improved reliability from a new API key was established.

## Investigation evidence and implementation

Before changing the adapter, eight repeated real binding calls and three
requests to the deployed public endpoint all succeeded for the original
approval question. No detailed event from the original failed request survived;
its exact cause remains unknown. These successful samples do not establish an
availability rate.

The regression loop is `cd public-demo && npm test`. Before the fix, injected
capacity failure and invalid-citation cases both returned `503` instead of
recovering (`503 !== 200`). After the change, 15 API/SQLite tests pass, including
single recovery, native timeout signals, refusal/exhaustion handling, concurrent
retry accounting, error references, and omission of sensitive error content.
These controlled failures verify recovery policy; they do not identify the
historical failure's cause.

The real local Workers HTTP runtime also answered the approval, public/full
application boundary, and undocumented pricing questions with the stricter
citation enum and native signal. Citation links were checked against actual
revision endpoints.
The pricing request produced a real `invalid_json` event on its first model
attempt; one counted retry returned a valid lack-of-evidence answer with no
invented price or citations. This demonstrates actual recovery from malformed
structured output, without establishing the original failed request's cause.

A Node `getPlatformProxy` wrapper cannot serialize an
`AbortSignal` into an `AI.run` RPC (`DevalueError`); use the actual Worker HTTP
runtime to test cancellation options. Do not remove production cancellation to
make that Node-only harness pass.
