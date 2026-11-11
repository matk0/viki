export const MODEL = '@cf/openai/gpt-oss-20b'
export const ATTEMPT_TIMEOUT_MS = 15000

export class InferenceError extends Error {
  constructor(kind, retryable = false, providerCode) {
    super(kind)
    this.kind = kind
    this.retryable = retryable
    this.providerCode = providerCode
  }
}

function providerFailure(error, signal) {
  // Workers AI prefixes upstream errors with an internal numeric code. Never
  // retain the rest of the message: it can contain request or provider data.
  const code = Number(String(error?.message ?? '').match(/^(\d{4}):/)?.[1]) || undefined
  if (code === 3036) return new InferenceError('provider_quota', false, code)
  if (signal?.aborted || [3007, 3008].includes(code)) return new InferenceError('provider_timeout', true, code)
  if (code === 3040) return new InferenceError('provider_capacity', true, code)
  return new InferenceError('provider_error', false, code)
}

export function finalAnswer(result, sources) {
  if (!result || typeof result !== 'object') throw new InferenceError('invalid_response')
  if (result.error) throw providerFailure({ message: String(result.error.code) + ':' })
  const output = Array.isArray(result.output) ? result.output : []
  if (result.incomplete_details?.reason === 'content_filter' || output.some((item) =>
    item?.type === 'message' && Array.isArray(item.content) && item.content.some((part) => part?.type === 'refusal'))) throw new InferenceError('answer_refused')
  if (result.incomplete_details || (result.status && result.status !== 'completed')) {
    throw new InferenceError('incomplete_output', result.incomplete_details?.reason === 'max_output_tokens')
  }
  const text = result.output_text ?? output.filter((item) => item?.type === 'message' && item.role === 'assistant')
    .flatMap((item) => Array.isArray(item.content) ? item.content : []).filter((item) => item?.type === 'output_text').map((item) => item.text).join('')
  if (typeof text !== 'string' || !text) throw new InferenceError('invalid_response')
  let value
  try { value = JSON.parse(text) }
  catch { throw new InferenceError('invalid_json', true) }
  const allowed = new Map(sources.map((source) => [source.sourceId, source]))
  if (!value || typeof value.answer !== 'string' || !value.answer.trim() || value.answer.length > 6000 ||
      !Array.isArray(value.source_ids) || value.source_ids.length > 6 ||
      Object.keys(value).some((key) => !['answer', 'source_ids'].includes(key))) throw new InferenceError('invalid_answer', true)
  if (value.source_ids.some((id) => !allowed.has(id))) throw new InferenceError('invalid_citation', true)
  return {
    answer: value.answer,
    citations: [...new Set(value.source_ids)].map((id) => {
      const detail = allowed.get(id)
      return { pageId: detail.page.id, revisionId: detail.repositoryRevision.id, pageTitle: detail.page.title, draft: false }
    }),
  }
}

export async function answerWithRetry(ai, { input, sources, reserveRetry, requestId }) {
  const schema = {
    type: 'object', additionalProperties: false, required: ['answer', 'source_ids'],
    properties: {
      answer: { type: 'string' },
      source_ids: { type: 'array', items: { type: 'string', enum: sources.map((source) => source.sourceId) }, maxItems: 6 },
    },
  }
  for (let attempt = 1; attempt <= 2; attempt++) {
    const signal = AbortSignal.timeout(ATTEMPT_TIMEOUT_MS)
    try {
      let result
      try {
        result = await ai.run(MODEL, {
          input, stream: false, max_output_tokens: 1536, reasoning: { effort: 'low' },
          text: { format: { type: 'json_schema', name: 'wiki_answer', strict: true, schema } },
        }, { signal })
      } catch (error) { throw providerFailure(error, signal) }
      return finalAnswer(result, sources)
    } catch (error) {
      if (!(error instanceof InferenceError)) throw error
      console.warn('qa_attempt_failed', { requestId, attempt, kind: error.kind, providerCode: error.providerCode })
      if (!error.retryable || attempt === 2) throw error
      // Every retry reserves another global model attempt before inference.
      await reserveRetry()
      await new Promise((resolve) => setTimeout(resolve, 250 + Math.floor(Math.random() * 250)))
    }
  }
}
