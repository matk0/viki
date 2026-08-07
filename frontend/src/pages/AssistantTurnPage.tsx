import { BookOpen, Check, Circle, FilePenLine, MessageCircleQuestion, Search, Sparkles, Square } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { AssistantDraftReceipt, Page } from '../api/types'
import { useAssistant, type AssistantTurnProgress } from '../assistant'
import { Markdown } from '../components/ui'
import { useI18n, type TranslationKey } from '../i18n'
import { Link, useRouter } from '../router'

const noDrafts: AssistantDraftReceipt[] = []

export function AssistantTurnPage({ turnId }: { turnId: string }) {
  const { t } = useI18n()
  const { navigate } = useRouter()
  const assistant = useAssistant()
  const [redirecting, setRedirecting] = useState(false)
  const [redirectError, setRedirectError] = useState('')
  const turn = assistant.turns[turnId] ?? recoverTurn(turnId, assistant.conversation)
  const drafts = turn?.drafts ?? noDrafts

  useEffect(() => {
    if (turn?.status !== 'completed' || drafts.length === 0) return
    let cancelled = false
    const timer = window.setTimeout(() => {
      setRedirecting(true)
      void api.pages()
        .then(({ pages }) => {
          if (!cancelled) navigate(draftDestination(drafts, pages), true)
        })
        .catch(() => {
          if (!cancelled) {
            setRedirecting(false)
            setRedirectError(t('assistant.turn.failed'))
          }
        })
    }, 850)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [drafts, navigate, t, turn?.status])

  if (!turn) {
    return <div className="assistant-turn-page page-container"><div className="assistant-turn-error" role="alert">{t('assistant.turn.unavailable')}</div></div>
  }

  const active = turn.status === 'running'
  const clarification = assistant.clarification?.turnId === turnId

  return <div className={`assistant-turn-page page-container ${turn.status}`}>
    <header className="assistant-turn-heading">
      <div>
        <span className="assistant-turn-kicker">{t('assistant.edit')}</span>
        <h1>{t('assistant.turn.title')}</h1>
        <p>{t('assistant.turn.description')}</p>
      </div>
      {active && <button type="button" className="secondary-button assistant-turn-stop" onClick={() => void assistant.stop()}><Square size={14} />{t('assistant.stop')}</button>}
    </header>

    <section className="assistant-turn-stage" aria-live="polite">
      <div className="assistant-turn-animation" aria-hidden="true">
        <span className="assistant-turn-star"><img src="/assistant-stars.svg" alt="" /></span>
        <i /><i /><i />
      </div>
      <div className="assistant-turn-copy">
        <strong>{turnStatusLabel(turn, redirecting, t)}</strong>
        <span className="typing" aria-hidden="true"><i /><i /><i /></span>
      </div>
    </section>

    <div className="assistant-turn-grid">
      <section className="assistant-turn-timeline" aria-label={t('assistant.turn.title')}>
        {(turn.activities.length > 0 ? turn.activities : ['submitted']).map((state, index, activities) => {
          const done = turn.status === 'completed' || index < activities.length - 1
          return <div className={done ? 'done' : 'active'} key={state}>
            <span>{done ? <Check size={17} /> : activityIcon(state)}</span>
            <p>{t(activityTranslation(state))}</p>
          </div>
        })}
        {clarification && <div className="needs-input"><span><MessageCircleQuestion size={17} /></span><p>{t('assistant.turn.clarification')}</p></div>}
      </section>

      <div className="assistant-turn-results">
        {turn.summary.trim() && <section className="assistant-turn-summary">
          <h2>{t('assistant.turn.summary')}</h2>
          <Markdown>{turn.summary}</Markdown>
        </section>}

        {turn.drafts.length > 0 && <section className="assistant-turn-drafts">
          <h2>{t('assistant.turn.created')}</h2>
          <div>{turn.drafts.map((draft) => <Link key={draft.revisionId} to={`/page/${draft.pageId}?revision=${encodeURIComponent(draft.revisionId)}`}>
            <span><FilePenLine size={17} /></span><strong>{draft.pageTitle}</strong><Check size={17} />
          </Link>)}</div>
        </section>}
      </div>
    </div>

    {(turn.error || redirectError) && <div className="assistant-turn-error" role="alert">{turn.error || redirectError}</div>}
  </div>
}

function recoverTurn(turnId: string, conversation: ReturnType<typeof useAssistant>['conversation']): AssistantTurnProgress | undefined {
  if (!conversation) return undefined
  const messages = conversation.messages.filter((message) => message.role === 'assistant' && (
    message.id === `turn-${turnId}` || message.id === `${turnId}-assistant` || message.id.startsWith(`${turnId}-assistant-`)
  ))
  if (messages.length === 0) return undefined
  const status = conversation.state === 'running' || conversation.state === 'awaiting_clarification'
    ? conversation.state
    : conversation.state === 'stopped' || conversation.state === 'error' ? conversation.state : 'completed'
  return {
    id: turnId,
    mode: 'edit',
    status,
    activities: status === 'completed' ? ['submitted', 'drafted'] : ['submitted'],
    summary: messages.map((message) => message.content).filter(Boolean).join('\n\n'),
    drafts: deduplicateDrafts(messages.flatMap((message) => message.drafts)),
  }
}

function deduplicateDrafts(drafts: AssistantDraftReceipt[]): AssistantDraftReceipt[] {
  return drafts.filter((draft, index) => drafts.findIndex((candidate) => candidate.revisionId === draft.revisionId) === index)
}

export function draftDestination(drafts: AssistantDraftReceipt[], pages: Page[]): string {
  const pageByID = new Map(pages.map((page) => [page.id, page]))
  const preferred = (['feature', 'scenario', 'concept'] as const)
    .flatMap((kind) => drafts.filter((draft) => pageByID.get(draft.pageId)?.kind === kind))[0] ?? drafts[0]
  return preferred ? `/page/${preferred.pageId}?revision=${encodeURIComponent(preferred.revisionId)}` : '/features'
}

function activityTranslation(state: string): TranslationKey {
  if (['submitted', 'thinking'].includes(state)) return 'assistant.turn.activity.understanding'
  if (state === 'searching') return 'assistant.turn.activity.searching'
  if (state === 'searched') return 'assistant.turn.activity.searched'
  if (state === 'reading') return 'assistant.turn.activity.reading'
  if (state === 'read') return 'assistant.turn.activity.read'
  if (['drafting', 'editing', 'writing', 'applying'].includes(state)) return 'assistant.turn.activity.drafting'
  if (['drafted', 'awaiting_approval'].includes(state)) return 'assistant.turn.activity.drafted'
  if (state === 'clarification_answered') return 'assistant.turn.activity.continuing'
  return 'assistant.turn.activity.working'
}

function activityIcon(state: string) {
  if (state === 'searching') return <Search size={17} />
  if (state === 'reading') return <BookOpen size={17} />
  if (['drafting', 'editing', 'writing', 'applying'].includes(state)) return <FilePenLine size={17} />
  if (['submitted', 'thinking'].includes(state)) return <Sparkles size={17} />
  return <Circle size={12} />
}

function turnStatusLabel(turn: AssistantTurnProgress, redirecting: boolean, t: ReturnType<typeof useI18n>['t']): string {
  if (redirecting || turn.status === 'completed') return t('assistant.turn.redirecting')
  if (turn.status === 'awaiting_clarification') return t('assistant.turn.clarification')
  if (turn.status === 'stopped') return t('assistant.turn.stopped')
  if (turn.status === 'error') return turn.error || t('assistant.turn.failed')
  const state = turn.activities.at(-1) ?? 'submitted'
  return t(activityTranslation(state))
}
