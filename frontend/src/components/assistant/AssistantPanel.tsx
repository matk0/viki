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
import { useSlovakVoiceInput } from '../../voice'
import { VikiSelect } from '../VikiSelect'
import { Markdown, Spinner } from '../ui'

const modeLabels: Record<AssistantMode, string> = {
  qa: 'Otázky',
  edit: 'Úpravy',
}

export function AssistantPanel() {
  const assistant = useAssistant()
  const scrollRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const element = scrollRef.current
    if (!element) return
    if (typeof element.scrollTo === 'function') element.scrollTo({ top: element.scrollHeight, behavior: 'smooth' })
    else element.scrollTop = element.scrollHeight
  }, [assistant.conversation?.messages, assistant.activity, assistant.clarification])

  const conversation = assistant.conversation
  const active = conversation?.state === 'running' || conversation?.state === 'awaiting_clarification'
  const visibleActivity = assistant.activity ?? (active && conversation
    ? { state: conversation.state === 'awaiting_clarification' ? 'clarifying' : 'thinking', mode: conversation.lastMode }
    : null)
  const voice = useSlovakVoiceInput(assistant.composer, assistant.setComposer, active || !assistant.modeAvailable)
  const submit = (event: FormEvent) => {
    if (voice.listening) {
      event.preventDefault()
      return
    }
    void assistant.send(event)
  }

  return <div className="assistant-panel">
    <header className="assistant-header">
      <div><span className="assistant-mark"><Sparkles size={16} /></span><div><strong>viki asistent</strong><small>Odpovede a návrhy</small></div></div>
      <button className="icon-button" onClick={() => void assistant.createConversation()} disabled={active || !assistant.status?.[assistant.mode]?.ready} aria-label="Nový rozhovor"><Plus size={17} /></button>
    </header>

    {assistant.conversations.length > 0 && conversation && <ConversationSelector conversations={assistant.conversations} conversation={conversation} disabled={active} onSelect={(id) => void assistant.selectConversation(id)} />}

    <div className="assistant-controls">
      <div className="mode-switch">
        <button type="button" disabled={active} className={assistant.mode === 'qa' ? 'active' : ''} onClick={() => assistant.setMode('qa')}><MessageCircleQuestion size={15} />Otázky</button>
        <button type="button" disabled={active} className={assistant.mode === 'edit' ? 'active' : ''} onClick={() => assistant.setMode('edit')}><FilePenLine size={15} />Úpravy</button>
      </div>
    </div>

    {assistant.connection === 'reconnecting' && <ConnectionNotice onReconnect={assistant.reconnect} />}
    {assistant.connection === 'disconnected' && conversation && <ConnectionNotice onReconnect={assistant.reconnect} disconnected />}
    {!assistant.loading && !assistant.modeAvailable && <div className="assistant-unavailable" role="status"><strong>Asistent je momentálne nedostupný.</strong><span>Viki môžete naďalej používať bez asistenta.</span><button type="button" onClick={() => void assistant.refresh()}><RefreshCw size={13} />Skontrolovať spojenie</button></div>}

    <div className="chat-messages" ref={scrollRef}>
      {assistant.loading && !conversation && <Spinner label="Pripravujem asistenta…" />}
      {!assistant.loading && conversation && conversation.messages.length === 0 && <Welcome />}
      {!assistant.loading && !conversation && <div className="assistant-welcome"><Sparkles size={23} /><h3>Asistent čaká na spojenie</h3><p>Keď bude asistent dostupný, môžete začať nový rozhovor. Pojmy a scenáre zostávajú dostupné.</p></div>}
      {conversation?.messages.map((message) => <Message key={message.id} message={message} />)}
      {visibleActivity && <div className="assistant-activity" role="status"><span className="typing" aria-hidden="true"><i /><i /><i /></span>{activityLabel(visibleActivity.state, visibleActivity.mode)}</div>}
      {assistant.clarification && <form className="clarification-card" onSubmit={(event) => void assistant.respondToClarification(event)}>
        <strong>Potrebujem doplnenie</strong>
        <p>{assistant.clarification.message}</p>
        {assistant.clarification.choices && assistant.clarification.choices.length > 0 && <div className="clarification-choices">{assistant.clarification.choices.map((choice) => <button type="button" key={choice} onClick={() => assistant.setClarificationResponse(choice)}>{choice}</button>)}</div>}
        <label><span>Vaša odpoveď</span><textarea rows={2} value={assistant.clarificationResponse} onChange={(event) => assistant.setClarificationResponse(event.target.value)} /></label>
        <button type="submit" disabled={!assistant.clarificationResponse.trim()}>Pokračovať</button>
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
        placeholder={assistant.mode === 'qa' ? 'Opýtajte sa viki…' : 'Opíšte, čo má viki pridať alebo zmeniť…'}
      />
      <div>
        <span className={`composer-hint ${voice.error ? 'error' : ''}`} role={voice.listening || voice.error ? 'status' : undefined}>
          {voice.error || (voice.listening ? 'Počúvam po slovensky…' : 'Enter odosiela · Shift + Enter nový riadok')}
        </span>
        {active
          ? <button type="button" className="stop-button" onClick={() => void assistant.stop()}><Square size={13} />Zastaviť</button>
          : <span className="composer-actions">
            <button
              type="button"
              className={`voice-button ${voice.listening ? 'listening' : ''}`}
              disabled={!assistant.modeAvailable || !voice.supported}
              aria-label={voice.listening ? 'Zastaviť hlasový vstup' : 'Začať hlasový vstup'}
              title={voice.supported ? 'Hlasový vstup v slovenčine' : 'Tento prehliadač nepodporuje hlasový vstup'}
              onClick={voice.listening ? voice.stop : voice.start}
            >
              {voice.listening ? <Square size={14} /> : <Mic size={16} />}
            </button>
            <button className="send-button" disabled={!assistant.modeAvailable || voice.listening || !assistant.composer.trim()} aria-label="Odoslať"><Send size={16} /></button>
          </span>}
      </div>
    </form>}
  </div>
}

function ConversationSelector({ conversations, conversation, disabled, onSelect }: { conversations: AssistantConversationSummary[]; conversation: AssistantConversation; disabled: boolean; onSelect: (id: string) => void }) {
  const selectedIndex = Math.max(0, conversations.findIndex((item) => item.id === conversation.id))
  const selectedTitle = conversationTitle(conversation.title, conversation.createdAt, selectedIndex)
  return <VikiSelect
    className="chat-selector"
    ariaLabel={`Rozhovor: ${selectedTitle}`}
    listboxLabel="Rozhovory"
    value={conversation.id}
    options={conversations.map((item, index) => ({ value: item.id, label: conversationTitle(item.title, item.createdAt, index) }))}
    disabled={disabled}
    onChange={onSelect}
  />
}

function Welcome() {
  return <div className="assistant-welcome"><Sparkles size={23} /><h3>Čo potrebujete zachytiť?</h3><p>Opýtajte sa na firemné pravidlá alebo opíšte nový pojem či proces.</p></div>
}

function Message({ message }: { message: AssistantMessage }) {
  return <article className={`chat-message ${message.role}`}>
    <span className="message-author">{message.role === 'assistant' ? <><Sparkles size={13} />viki</> : 'Vy'}<em className={`message-mode ${message.mode}`}>{modeLabels[message.mode]}</em></span>
    <Markdown>{message.content}</Markdown>
    {message.drafts.map((revision) => <DraftCard revision={revision} key={revision.revisionId} />)}
    {message.citations.length > 0 && <CitationList citations={message.citations} />}
  </article>
}

function CitationList({ citations }: { citations: Citation[] }) {
  return <div className="citation-list">{citations.map((citation, index) => {
    const revisionLabel = `revízia ${citation.revisionId.slice(0, 8)}`
    return <span className="citation-link" title={`Presná revízia: ${citation.revisionId}`} key={citation.revisionId}><Link to={`/page/${citation.pageId}?revision=${encodeURIComponent(citation.revisionId)}`}><span>{index + 1}</span><strong>{citation.pageTitle}</strong><small>{revisionLabel}</small>{citation.draft && <em>Koncept</em>}</Link></span>
  })}</div>
}

function DraftCard({ revision }: { revision: AssistantDraftReceipt }) {
  return <Link className="created-draft" to={`/page/${revision.pageId}?revision=${encodeURIComponent(revision.revisionId)}`}><Check size={15} /><span><strong>Koncept vytvorený</strong><b>{revision.pageTitle}</b><small title={`Presná revízia: ${revision.revisionId}`}>revízia {revision.revisionId.slice(0, 8)}</small></span></Link>
}

function ConnectionNotice({ onReconnect, disconnected = false }: { onReconnect: () => void; disconnected?: boolean }) {
  return <div className="assistant-connection" role="status"><RefreshCw size={13} /><span>{disconnected ? 'Spojenie s asistentom sa prerušilo.' : 'Spojenie s asistentom sa obnovuje…'}</span><button type="button" onClick={onReconnect}>Pripojiť znova</button></div>
}

function conversationTitle(title: string | undefined, createdAt: string, index: number): string {
  if (title?.trim()) return title
  const date = new Date(createdAt)
  if (!Number.isNaN(date.valueOf())) return `Rozhovor · ${date.toLocaleDateString('sk-SK')}`
  return `Rozhovor ${index + 1}`
}

function activityLabel(state: string, mode: AssistantMode): string {
  if (['searching', 'reading', 'retrieving'].includes(state)) return 'Hľadám vo viki…'
  if (['drafting', 'editing', 'applying', 'writing'].includes(state)) return 'Pripravujem návrh…'
  if (state === 'awaiting_approval') return 'Návrh čaká na schválenie…'
  if (['clarifying', 'waiting_for_clarification'].includes(state)) return 'Čakám na doplnenie…'
  if (state === 'stopping') return 'Zastavujem…'
  if (state === 'submitting') return 'Odosielam…'
  return mode === 'edit' ? 'Premýšľam nad úpravou…' : 'Hľadám odpoveď…'
}
