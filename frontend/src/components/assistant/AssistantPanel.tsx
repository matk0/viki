import {
  Check,
  FilePenLine,
  MessageCircleQuestion,
  Mic,
  Plus,
  RefreshCw,
  Send,
  Sparkles,
  Square,
} from 'lucide-react'
import { useEffect, useRef, type FormEvent } from 'react'
import type { AssistantConversation, AssistantConversationSummary, AssistantDraftReceipt, AssistantMessage, AssistantMode, Citation } from '../../api/types'
import { useAssistant } from '../../assistant'
import { Link } from '../../router'
import { VikiSelect } from '../VikiSelect'
import { Markdown, Spinner } from '../ui'
import { useI18n } from '../../i18n'

export function AssistantPanel() {
  const { t } = useI18n()
  const assistant = useAssistant()
  const scrollRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const element = scrollRef.current!
    if (typeof element.scrollTo === 'function') element.scrollTo({ top: element.scrollHeight, behavior: 'smooth' })
    else element.scrollTop = element.scrollHeight
  }, [assistant.conversation?.messages, assistant.activity, assistant.clarification])

  const conversation = assistant.conversation
  const active = conversation?.state === 'running' || conversation?.state === 'awaiting_clarification'
  const visibleActivity = assistant.activity ?? (active && conversation
    ? { state: conversation.state === 'awaiting_clarification' ? 'clarifying' : 'thinking', mode: conversation.lastMode }
    : null)
  const voice = assistant.voice
  const submit = (event: FormEvent) => {
    if (voice.listening) {
      event.preventDefault()
      return
    }
    void assistant.send(event)
  }

  return <div className="assistant-panel">
    <header className="assistant-header">
      <div><span className="assistant-mark"><Sparkles size={16} /></span><div><strong>{t('assistant.name')}</strong><small>{t('assistant.subtitle')}</small></div></div>
      <button className="icon-button" onClick={() => void assistant.createConversation()} disabled={active || !assistant.status?.[assistant.mode]?.ready} aria-label={t('assistant.newConversation')}><Plus size={17} /></button>
    </header>

    {assistant.conversations.length > 0 && conversation && <ConversationSelector conversations={assistant.conversations} conversation={conversation} disabled={active} onSelect={(id) => void assistant.selectConversation(id)} />}

    <div className="assistant-controls">
      <div className="mode-switch">
        <button type="button" disabled={active} className={assistant.mode === 'qa' ? 'active' : ''} onClick={() => assistant.setMode('qa')}><MessageCircleQuestion size={15} />{t('assistant.qa')}</button>
        <button type="button" disabled={active} className={assistant.mode === 'edit' ? 'active' : ''} onClick={() => assistant.setMode('edit')}><FilePenLine size={15} />{t('assistant.edit')}</button>
      </div>
    </div>

    {assistant.connection === 'reconnecting' && <ConnectionNotice onReconnect={assistant.reconnect} />}
    {assistant.connection === 'disconnected' && conversation && <ConnectionNotice onReconnect={assistant.reconnect} disconnected />}
    {!assistant.loading && !assistant.modeAvailable && <div className="assistant-unavailable" role="status"><strong>{t('assistant.unavailable')}</strong><span>{t('assistant.coreAvailable')}</span><button type="button" onClick={() => void assistant.refresh()}><RefreshCw size={13} />{t('assistant.checkConnection')}</button></div>}

    <div className="chat-messages" ref={scrollRef}>
      {assistant.loading && !conversation && <Spinner label={t('assistant.preparing')} />}
      {!assistant.loading && conversation && conversation.messages.length === 0 && <Welcome />}
      {!assistant.loading && !conversation && <div className="assistant-welcome"><Sparkles size={23} /><h3>{t('assistant.waiting')}</h3><p>{t('assistant.waitingBody')}</p></div>}
      {conversation?.messages.map((message) => <Message key={message.id} message={message} />)}
      {visibleActivity && <div className="assistant-activity" role="status"><span className="typing" aria-hidden="true"><i /><i /><i /></span>{activityLabel(visibleActivity.state, visibleActivity.mode, t)}</div>}
      {assistant.clarification && <form className="clarification-card" onSubmit={(event) => void assistant.respondToClarification(event)}>
        <strong>{t('assistant.needClarification')}</strong>
        <p>{assistant.clarification.message}</p>
        {assistant.clarification.choices && assistant.clarification.choices.length > 0 && <div className="clarification-choices">{assistant.clarification.choices.map((choice) => <button type="button" key={choice} onClick={() => assistant.setClarificationResponse(choice)}>{choice}</button>)}</div>}
        <label><span>{t('assistant.yourAnswer')}</span><textarea rows={2} value={assistant.clarificationResponse} onChange={(event) => assistant.setClarificationResponse(event.target.value)} /></label>
        <button type="submit" disabled={!assistant.clarificationResponse.trim()}>{t('assistant.continue')}</button>
      </form>}
      {assistant.error && <div className="assistant-error" role="alert">{assistant.error}</div>}
    </div>

    {conversation && <form className="assistant-composer" onSubmit={submit}>
      <textarea
        rows={2}
        value={assistant.composer}
        disabled={active || !assistant.modeAvailable}
        onChange={(event) => assistant.setComposer(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault()
            event.currentTarget.form?.requestSubmit()
          }
        }}
        placeholder={assistant.mode === 'qa' ? t('assistant.askPlaceholder') : t('assistant.editPlaceholder')}
      />
      <div>
        <span className={`composer-hint ${voice.error ? 'error' : ''}`} role={voice.listening || voice.error ? 'status' : undefined}>
          {voice.error || (voice.listening ? t('assistant.listening') : t('assistant.shortcut'))}
        </span>
        {active
          ? <button type="button" className="stop-button" onClick={() => void assistant.stop()}><Square size={13} />{t('assistant.stop')}</button>
          : <span className="composer-actions">
            <button
              type="button"
              className={`voice-button ${voice.listening ? 'listening' : ''}`}
              disabled={!assistant.modeAvailable || !voice.supported}
              aria-label={voice.listening ? t('assistant.stopVoice') : t('assistant.startVoice')}
              title={voice.supported ? t('assistant.voiceTitle') : t('assistant.voiceUnsupported')}
              onClick={voice.listening ? voice.stop : voice.start}
            >
              {voice.listening ? <Square size={14} /> : <Mic size={16} />}
            </button>
            <button className="send-button" disabled={!assistant.modeAvailable || voice.listening || !assistant.composer.trim()} aria-label={t('assistant.send')}><Send size={16} /></button>
          </span>}
      </div>
    </form>}
  </div>
}

function ConversationSelector({ conversations, conversation, disabled, onSelect }: { conversations: AssistantConversationSummary[]; conversation: AssistantConversation; disabled: boolean; onSelect: (id: string) => void }) {
  const { locale, t } = useI18n()
  const selectedIndex = Math.max(0, conversations.findIndex((item) => item.id === conversation.id))
  const selectedTitle = conversationTitle(conversation.title, conversation.createdAt, selectedIndex, locale, t)
  return <VikiSelect
    className="chat-selector"
    ariaLabel={t('assistant.conversation', { title: selectedTitle })}
    listboxLabel={t('assistant.conversations')}
    value={conversation.id}
    options={conversations.map((item, index) => ({ value: item.id, label: conversationTitle(item.title, item.createdAt, index, locale, t) }))}
    disabled={disabled}
    onChange={onSelect}
  />
}

function Welcome() {
  const { t } = useI18n()
  return <div className="assistant-welcome"><Sparkles size={23} /><h3>{t('assistant.welcome')}</h3><p>{t('assistant.welcomeBody')}</p></div>
}

function Message({ message }: { message: AssistantMessage }) {
  const { t } = useI18n()
  return <article className={`chat-message ${message.role}`}>
    <span className="message-author">{message.role === 'assistant' ? <><Sparkles size={13} />viki</> : t('assistant.you')}<em className={`message-mode ${message.mode}`}>{message.mode === 'qa' ? t('assistant.qa') : t('assistant.edit')}</em></span>
    <Markdown>{message.content}</Markdown>
    {message.drafts.map((revision) => <DraftCard revision={revision} key={revision.revisionId} />)}
    {message.citations.length > 0 && <CitationList citations={message.citations} />}
  </article>
}

function CitationList({ citations }: { citations: Citation[] }) {
  const { t } = useI18n()
  return <div className="citation-list">{citations.map((citation, index) => {
    const revisionLabel = t('assistant.revision', { id: citation.revisionId.slice(0, 8) })
    return <span className="citation-link" title={t('assistant.exactRevision', { id: citation.revisionId })} key={citation.revisionId}><Link to={`/page/${citation.pageId}?revision=${encodeURIComponent(citation.revisionId)}`}><span>{index + 1}</span><strong>{citation.pageTitle}</strong><small>{revisionLabel}</small>{citation.draft && <em>{t('status.draft')}</em>}</Link></span>
  })}</div>
}

function DraftCard({ revision }: { revision: AssistantDraftReceipt }) {
  const { t } = useI18n()
  return <Link className="created-draft" to={`/page/${revision.pageId}?revision=${encodeURIComponent(revision.revisionId)}`}><Check size={15} /><span><strong>{t('assistant.draftCreated')}</strong><b>{revision.pageTitle}</b><small title={t('assistant.exactRevision', { id: revision.revisionId })}>{t('assistant.revision', { id: revision.revisionId.slice(0, 8) })}</small></span></Link>
}

function ConnectionNotice({ onReconnect, disconnected = false }: { onReconnect: () => void; disconnected?: boolean }) {
  const { t } = useI18n()
  return <div className="assistant-connection" role="status"><RefreshCw size={13} /><span>{disconnected ? t('assistant.disconnected') : t('assistant.reconnecting')}</span><button type="button" onClick={onReconnect}>{t('assistant.reconnect')}</button></div>
}

function conversationTitle(title: string | undefined, createdAt: string, index: number, locale: 'sk' | 'en', t: ReturnType<typeof useI18n>['t']): string {
  if (title?.trim()) return title
  const date = new Date(createdAt)
  if (!Number.isNaN(date.valueOf())) return t('assistant.conversationDated', { date: date.toLocaleDateString(locale === 'en' ? 'en-GB' : 'sk-SK') })
  return t('assistant.conversationNumber', { number: index + 1 })
}

function activityLabel(state: string, mode: AssistantMode, t: ReturnType<typeof useI18n>['t']): string {
  if (['searching', 'reading', 'retrieving'].includes(state)) return t('assistant.activity.searching')
  if (['drafting', 'editing', 'applying', 'writing'].includes(state)) return t('assistant.activity.drafting')
  if (state === 'awaiting_approval') return t('assistant.activity.awaiting')
  if (['clarifying', 'waiting_for_clarification'].includes(state)) return t('assistant.activity.clarifying')
  if (state === 'stopping') return t('assistant.activity.stopping')
  if (state === 'submitting') return t('assistant.activity.submitting')
  return mode === 'edit' ? t('assistant.activity.editing') : t('assistant.activity.answering')
}
