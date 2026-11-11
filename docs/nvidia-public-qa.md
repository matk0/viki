# A free catalogue model for public Viki Q&A

Assessed 11 November 2026. Model recommendation and integration guidance; no inference, account changes, or provider terms acceptance were performed during this research.

## Recommended model and host

Use **OpenAI gpt-oss-20b**, which is currently listed as a **Free Endpoint in NVIDIA's catalogue**, hosted through **Cloudflare Workers AI** as `@cf/openai/gpt-oss-20b`. This keeps the selected model within the user-linked catalogue while keeping public-demo infrastructure and inference on Cloudflare. It does not use NVIDIA's hosted trial endpoint. [NVIDIA catalogue entry](https://build.nvidia.com/openai/gpt-oss-20b), [current catalogue search](https://build.nvidia.com/models?q=gpt-oss), [Cloudflare model](https://developers.cloudflare.com/workers-ai/models/gpt-oss-20b/)

The model is designed for lower latency and specialized applications. Cloudflare advertises a 128,000-token context window, function calling, configurable reasoning, and zero data retention for its hosted version. OpenAI releases the model under Apache 2.0. These properties make it a reasonable starting point for answering short questions from a small, explicitly supplied set of public Viki documents. This suitability is an engineering judgment, not a measured comparison on Viki's questions. Do not send the entire maximum context window per question. [Cloudflare model information](https://developers.cloudflare.com/workers-ai/models/gpt-oss-20b/), [official model repository](https://github.com/openai/gpt-oss)

No automatic alternate model is recommended initially. Keeping one model makes quality, usage, and failure behavior easier to evaluate. When inference is unavailable or the allowance is exhausted, leave the reader/search available and pause Q&A visibly.

## Why the inference provider matters

NVIDIA's catalogue-linked **API Trial Terms**, version 19 September 2025, grant limited trial access in section 1.2. Section 1.4 describes internal testing/evaluation without a separate production subscription; section 4.2 restricts making the service or generated content available to others. These terms do not establish permission to run an unattended public Q&A site using the free hosted trial. The catalogue's trial disclaimer also says inputs/outputs are recorded and warns against submitting confidential or personal information. Do not equate a model's open license with unrestricted use of a particular hosted endpoint. [Catalogue-linked trial terms, sections 1.2, 1.4, 3.3, 4.2](https://assets.ngc.nvidia.com/products/api-catalog/legal/NVIDIA%20API%20Trial%20Terms%20of%20Service.pdf)

A different NVIDIA subscription or express permission could change that assessment, but none was verified. The public catalogue did not establish the current account's API credit balance, trial duration, or exact rate limit; this research does not promise a universal free-token or requests-per-minute allowance. NVIDIA's own RAG troubleshooting documentation still describes exhausted API Catalogue credits as requiring additional credits. [NVIDIA credit troubleshooting](https://docs.nvidia.com/rag/latest/troubleshooting.html)

Cloudflare's hosted copy is a separate provider route governed by its applicable service terms and the model license. It needs no NVIDIA API key. Keep inference inside a server-side Workers AI binding; visitors receive answers, not Cloudflare credentials. [Workers AI binding](https://developers.cloudflare.com/workers-ai/configuration/bindings/), [model license](https://github.com/openai/gpt-oss)

## Free allowance and practical budget

Workers AI provides **10,000 neurons per account per day** on Workers Free. The allowance resets at 00:00 UTC; additional operations fail after the Free allowance is exhausted. GPT-OSS-20B is not among the models that the current pricing page requires paid billing to access. Its listed consumption is 18,182 neurons per million input tokens and 27,273 per million output tokens. [Workers AI pricing](https://developers.cloudflare.com/workers-ai/platform/pricing/)

For illustration, 4,000 input tokens plus 600 output tokens consumes approximately `4000 × 18182 / 1000000 + 600 × 27273 / 1000000 = 89.1` neurons. That implies roughly 112 such calls from the full allowance, before any other account usage. This is a conditional calculation, not a daily-question guarantee: reasoning, larger history/context, retries, and other inference consume capacity too. Bound total generated tokens and measure real usage. Keep the account on Free; do not automatically upgrade or switch to a billed endpoint.

Use application counters with a conservative global daily ceiling and per-visitor/IP limits in addition to provider limits. Make one bounded synchronous inference per question, use only the selected public catalogue, and retain conversations only in the individual visitor's browser if persistence is unnecessary. A request-count ceiling is not itself a precise neuron meter.

## Exact Workers AI request and response contracts

The generated model page's simple JSON schema is incomplete: it exposes chat-style fields, while its synchronous output schema only says `object`. The current official Workers runtime definitions give this model **mutually exclusive Responses and Chat Completions inputs/outputs**. They explicitly support low/medium/high reasoning for both interfaces and require reasoning to remain enabled. Use one interface consistently. [Current runtime types](https://github.com/cloudflare/workerd/blob/main/types/defines/ai.d.ts), [model-specific type tests](https://github.com/cloudflare/workerd/blob/main/types/test/types/ai-reasoning.ts), [published model metadata](https://github.com/cloudflare/cloudflare-docs/blob/production/src/content/workers-ai-models/gpt-oss-20b.json)

For the documented Responses binding route, the current types support:

```ts
const result = await env.AI.run('@cf/openai/gpt-oss-20b', {
  input: [
    { role: 'system', content: instructionsAndSelectedPublicSources },
    ...boundedVisitorHistory,
    { role: 'user', content: question },
  ],
  stream: false,
  max_output_tokens: 1024,
  reasoning: { effort: 'low' },
  text: {
    format: {
      type: 'json_schema',
      name: 'wiki_answer',
      strict: true,
      schema: answerSchema,
    },
  },
});
```

`answerSchema` should require a short answer and an array of supplied source IDs, with no additional properties. The proposed token bound is an initial application choice, not a measured optimum. Responses output is `output_text` and/or `output[]` message items containing `content[]` entries of type `output_text` with a `text` string. Check completion/error/incomplete state, collect only final message text, then parse and validate the answer. Do not display reasoning items or treat a generic `{response: string}` shape as the established contract for this model. [Responses binding announcement](https://developers.cloudflare.com/changelog/2025-08-05-openai-open-models/), [current output definitions](https://github.com/cloudflare/workerd/blob/main/types/defines/ai.d.ts)

The current types also support Chat Completions through `messages`, `reasoning_effort: 'low'`, `max_completion_tokens`, `stream: false`, and `response_format: {type: 'json_schema', json_schema: {name, strict, schema}}`. That interface returns `choices[].message.content` with `finish_reason`, rather than Responses `output[]`. Never combine `input` with `messages`, or Responses token/reasoning/format fields with Chat Completions fields.

Cloudflare's JSON Mode guide warns that schema requests can fail and says JSON Mode does not stream. Its older supported-model list does not enumerate GPT-OSS, while current runtime types/model metadata include structured-output fields. Therefore a real authenticated call must verify this selected interface and schema before public Q&A is considered working. Do not silently retry indefinitely or switch to free-form output after schema failure. [JSON Mode](https://developers.cloudflare.com/workers-ai/features/json-mode/), [model schemas](https://developers.cloudflare.com/workers-ai/models/gpt-oss-20b/)

## Acceptance checks for the demo

- Ask factual questions spanning concepts, review rules, imports, translations, and the boundary between the demo and full Viki. Answers must cite only the sources actually supplied for that request.
- Ask an unanswered question and a prompt requesting invented facts or external URLs. Require a clear lack-of-evidence answer; reject unknown citation IDs server-side. Map allowed IDs to source links in application code.
- Verify two independent browsers have separate histories; no visitor-supplied assistant message may establish authority over the catalogue or server instructions.
- Verify truncated output, provider rejection, invalid JSON, unknown citations, and exhausted allowance produce a visible Q&A error while browsing/search remain available.
- Measure latency, total token usage including reasoning, and English/Slovak answer quality on the actual free endpoint. These have not been tested in this research.

The catalogue and Cloudflare model pages were inspected in Chrome; current official repository sources supplied precise types, license, and billing evidence. No NVIDIA trial terms were accepted and no NVIDIA hosted inference was invoked.

## Implementation validation

The implemented adapter was tested against the real Workers AI binding using the Responses interface above. Four public-document questions completed successfully: approval blockers, automatic deployment, differences between the demo and the full application, and undocumented enterprise pricing. Returned source IDs were validated server-side. The price question returned a lack-of-evidence answer without invented pricing or citations.

These smoke calls took approximately 0.8–2.6 seconds and consumed 17.6–26.3 neurons each. The first approval answer cited both the parent-feature rule and the unresolved-objection rule. These are observed examples, not a comparative benchmark or a latency/quality guarantee. Local API tests mock inference; the Google Chrome tests run the real local Workers runtime with model responses intercepted.
