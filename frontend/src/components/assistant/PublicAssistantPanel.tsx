import { useState, type FormEvent } from 'react'
import { Send, Trash2, X } from 'lucide-react'
import type { Citation } from '../../api/types'
import { useWorkspace } from '../../workspace'
import { Link } from '../../router'
import { Markdown } from '../ui'

interface Message {
  role: 'user' | 'assistant'
  content: string
  citations: Citation[]
}

export function PublicAssistantPanel() {
  const { setAssistantOpen } = useWorkspace()
  const [messages, setMessages] = useState<Message[]>([])
  const [question, setQuestion] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const content = question.trim()
    if (!content || pending) return
    const history = messages.slice(-4).map((message) => ({ role: message.role, content: message.content.slice(0, 800) }))
    setMessages((current) => [...current, { role: 'user', content, citations: [] }])
    setQuestion('')
    setError('')
    setPending(true)
    try {
      const response = await fetch('/api/public/qa', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ question: content, history }),
      })
      const result = await response.json()
      if (!response.ok) throw new Error(result.error.message)
      setMessages((current) => [...current, { role: 'assistant', content: result.answer, citations: result.citations }])
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'The answer could not be loaded.')
    } finally { setPending(false) }
  }
  return <section className="public-qa" aria-label="Ask about Viki">
    <header className="public-qa-heading">
      <div><h2>Ask about Viki</h2><p>Answers from this documentation, with sources.</p></div>
      <button className="icon-button" onClick={() => setAssistantOpen(false)} aria-label="Close assistant"><X size={18} /></button>
    </header>
    <div className="public-qa-messages" aria-live="polite">
      {messages.length === 0 && <div className="public-qa-intro">
        <h3>Let Viki explain itself.</h3>
        <p>Try “What blocks approval?” or “How does importing from Git work?”</p>
      </div>}
      {messages.map((message, index) => <article className={'public-qa-message ' + message.role} key={index}>
        <strong>{message.role === 'user' ? 'You' : 'Viki'}</strong>
        <Markdown>{message.content}</Markdown>
        {message.citations.length > 0 && <nav aria-label="Answer sources" className="public-qa-sources">
          {message.citations.map((citation) => <Link key={citation.revisionId} to={'/page/' + citation.pageId + '?revision=' + citation.revisionId}>{citation.pageTitle}</Link>)}
        </nav>}
      </article>)}
      {pending && <p role="status">Reading the documentation…</p>}
      {error && <p role="alert" className="form-error">{error}</p>}
    </div>
    <form onSubmit={(event) => void submit(event)} className="public-qa-form">
      <label htmlFor="public-question">Your question</label>
      <textarea id="public-question" value={question} onChange={(event) => setQuestion(event.target.value)} maxLength={1000} rows={3} placeholder="What would you like to understand?" />
      <div className="public-qa-actions">
        <button className="secondary-button" type="button" disabled={pending || messages.length === 0} onClick={() => { setMessages([]); setError('') }}><Trash2 size={14} />New chat</button>
        <button className="primary-button" disabled={pending || !question.trim()}><Send size={14} />Ask</button>
      </div>
      <p className="public-qa-note">History stays in this tab and clears on reload. Questions and recent messages are sent to Cloudflare AI. Free daily usage is limited.</p>
    </form>
  </section>
}
