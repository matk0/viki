import { useEffect, useState, type FormEvent } from 'react'
import { Check, Circle, FilePlus2, Pencil, X } from 'lucide-react'
import { useAssistant } from '../assistant'
import { APIError, api } from '../api/client'
import type { AssistantChangeOperation, AssistantDraftProposal, Page } from '../api/types'
import { Markdown } from '../components/ui'
import { useWorkspace } from '../workspace'

const activityLabels: Record<string, string> = {
  submitting: 'Odovzdávam zadanie',
  thinking: 'Rozumiem požiadavke',
  searching: 'Hľadám súvislosti vo viki',
  reading: 'Čítam existujúce pravidlá',
  drafting: 'Skladám návrh zmien',
  editing: 'Skladám návrh zmien',
  writing: 'Skladám návrh zmien',
  applying: 'Kontrolujem návrh',
  awaiting_approval: 'Čaká na vaše schválenie',
}

export function DraftPage({ proposalId }: { proposalId: string }) {
  const assistant = useAssistant()
  const { pages, reloadPages } = useWorkspace()
  const streamed = assistant.proposals[proposalId]
  const [proposal, setProposal] = useState<AssistantDraftProposal | null>(streamed ?? null)
  const [loadingAction, setLoadingAction] = useState<'approve' | 'reject' | null>(null)
  const [rejectOpen, setRejectOpen] = useState(false)
  const [rejectionReason, setRejectionReason] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (streamed) setProposal(streamed)
  }, [streamed])

  useEffect(() => {
    if (proposal || assistant.error) return
    let cancelled = false
    let timer: number | undefined
    const load = async () => {
      try {
        const next = await api.draftProposal(proposalId)
        if (!cancelled) setProposal(next)
      } catch (reason) {
        if (cancelled) return
        if (reason instanceof APIError ? reason.status === 404 : (reason as { status?: number })?.status === 404) {
          timer = window.setTimeout(() => void load(), 750)
          return
        }
        setError(reason instanceof Error ? reason.message : 'Návrh sa nepodarilo načítať.')
      }
    }
    void load()
    return () => {
      cancelled = true
      if (timer) window.clearTimeout(timer)
    }
  }, [assistant.error, proposal, proposalId])

  const approve = async () => {
    setLoadingAction('approve')
    setError('')
    try {
      const published = await api.approveDraftProposal(proposalId)
      setProposal(published)
      await reloadPages()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Návrh sa nepodarilo publikovať.')
    } finally {
      setLoadingAction(null)
    }
  }

  const reject = async (event: FormEvent) => {
    event.preventDefault()
    const reason = rejectionReason.trim()
    if (!reason) return
    setLoadingAction('reject')
    setError('')
    try {
      setProposal(await api.discardDraftProposal(proposalId, reason))
      setRejectOpen(false)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Návrh sa nepodarilo odmietnuť.')
    } finally {
      setLoadingAction(null)
    }
  }

  return <div className="draft-page page-container">
    <header className="draft-page-heading">
      <h1>Koncept</h1>
      <p>{proposal?.summary ?? 'Viki práve skladá z vašej požiadavky konkrétne zmeny.'}</p>
    </header>

    {!proposal && !assistant.error && <GeneratingDraft state={assistant.activity?.state} />}

    {proposal && <>
      <section className="proposal-operations" aria-label="Navrhované zmeny">
        {groupProposalOperations(proposal.operations).map((group, groupIndex) => <div className="proposal-operation-group" key={operationKey(group.parent, groupIndex)}>
          <OperationPreview operation={group.parent} operations={proposal.operations} pages={pages} />
          {group.children.length > 0 && <div className="proposal-operation-children" aria-label={`Nové pojmy pre ${group.parent.content.title}`}>
            {group.children.map((child, childIndex) => <div className="proposal-operation-child" key={operationKey(child, childIndex)}>
              <OperationPreview operation={child} operations={proposal.operations} pages={pages} />
            </div>)}
          </div>}
        </div>)}
      </section>

      {proposal.status === 'awaiting_approval' && <footer className="proposal-approval">
        <div>
          <strong>Publikovať {acceptedRevisionCount(proposal.operations.length)}?</strong>
          <p>Schválenie vykoná všetky zmeny naraz. Ak sa niektorá nedá bezpečne publikovať, nevytvorí sa nič.</p>
        </div>
        <div className="proposal-actions">
          <button className="secondary-button reject-button" disabled={loadingAction !== null} onClick={() => setRejectOpen(true)}><X size={15} />Odmietnuť</button>
          <button className="primary-button" disabled={loadingAction !== null} onClick={() => void approve()}><Check size={15} />{loadingAction === 'approve' ? 'Publikujem…' : 'Schváliť a publikovať'}</button>
        </div>
      </footer>}

      {proposal.status === 'published' && proposal.publishedRevisions.length > 0 && <div className="published-links">
        {proposal.publishedRevisions.map((revision) => <a key={revision.id} href={`/page/${revision.pageId}?revision=${revision.id}`}>{revision.title}<span>otvoriť prijatú revíziu →</span></a>)}
      </div>}

      {proposal.status === 'awaiting_approval' && rejectOpen && <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && loadingAction === null && setRejectOpen(false)}>
        <form className="modal-card proposal-rejection-dialog" role="dialog" aria-modal="true" aria-labelledby="proposal-rejection-title" onSubmit={(event) => void reject(event)}>
          <div className="modal-heading">
            <div><span className="eyebrow">Kontrola návrhu</span><h2 id="proposal-rejection-title">Odmietnuť návrh?</h2></div>
            <button type="button" className="icon-button" disabled={loadingAction !== null} onClick={() => setRejectOpen(false)} aria-label="Zavrieť"><X size={18} /></button>
          </div>
          <p className="proposal-rejection-copy">Napíšte komentár, prečo návrh odmietate. Vo viki sa nevytvorí žiadny záznam.</p>
          <label>Dôvod odmietnutia<textarea autoFocus required maxLength={2000} rows={5} value={rejectionReason} onChange={(event) => setRejectionReason(event.target.value)} placeholder="Čo treba v návrhu opraviť alebo doplniť?" /></label>
          {error && <div className="form-error" role="alert">{error}</div>}
          <div className="modal-actions"><button type="button" className="secondary-button" disabled={loadingAction !== null} onClick={() => setRejectOpen(false)}>Zrušiť</button><button className="primary-button rejection-button" disabled={loadingAction !== null || !rejectionReason.trim()}>{loadingAction === 'reject' ? 'Odmietam…' : 'Odmietnuť návrh'}</button></div>
        </form>
      </div>}
    </>}

    {((error && !rejectOpen) || (!proposal && assistant.error)) && <div className="form-error" role="alert">{error || assistant.error}</div>}
  </div>
}

function GeneratingDraft({ state }: { state?: string }) {
  const activeLabel = activityLabels[state ?? 'thinking'] ?? 'Viki pripravuje návrh'
  return <section className="draft-generating" aria-live="polite">
    <div className="draft-orbit"><span /><span /><span /></div>
    <div><h2>Viki pripravuje návrh</h2><p>{activeLabel}…</p></div>
    <ol>
      <li className="done"><Check size={14} />Zadanie prijaté</li>
      <li className="active"><span className="spinner" />Príprava zmien</li>
      <li><Circle size={11} />Vaše schválenie</li>
    </ol>
  </section>
}

interface DisplayOperationGroup {
  parent: AssistantChangeOperation
  children: AssistantChangeOperation[]
}

function groupProposalOperations(operations: AssistantChangeOperation[]): DisplayOperationGroup[] {
  const newPrimitives = new Map(
    operations
      .filter((operation) => operation.operation === 'create' && operation.kind === 'primitive' && operation.clientKey)
      .map((operation) => [operation.clientKey!, operation]),
  )
  const nestedPrimitiveKeys = new Set<string>()
  const groups = operations
    .filter((operation) => operation.kind !== 'primitive')
    .map((parent) => {
      const children = parent.content.references.flatMap((reference) => {
        const key = reference.targetClientKey
        if (!key || nestedPrimitiveKeys.has(key)) return []
        const primitive = newPrimitives.get(key)
        if (!primitive) return []
        nestedPrimitiveKeys.add(key)
        return [primitive]
      })
      return { parent, children }
    })

  for (const operation of operations) {
    if (operation.kind !== 'primitive' || (operation.clientKey && nestedPrimitiveKeys.has(operation.clientKey))) continue
    groups.push({ parent: operation, children: [] })
  }

  return groups
}

function operationKey(operation: AssistantChangeOperation, index: number): string {
  return operation.clientKey || operation.pageId || String(index)
}

function OperationPreview({ operation, operations, pages }: { operation: AssistantChangeOperation; operations: AssistantChangeOperation[]; pages: Page[] }) {
  const kind = operation.kind === 'primitive' ? 'Pojem' : operation.kind === 'scenario' ? 'Scenár' : 'Podscenár'
  const primitiveReferences = operation.kind === 'primitive' ? [] : operation.content.references.flatMap((reference) => {
    const resolved = resolvePrimitiveReference(reference, operations, pages)
    return resolved ? [resolved] : []
  })
  return <article className="proposal-operation" id={operation.clientKey ? draftOperationAnchor(operation.clientKey) : undefined}>
    <header>
      <span>{operation.operation === 'create' ? <FilePlus2 size={15} /> : <Pencil size={15} />}{operation.operation === 'create' ? 'Vytvoriť' : 'Upraviť'} · {kind}</span>
      <h2>{operation.content.title}</h2>
      <small>/{operation.slug}</small>
    </header>
    {operation.content.bodyMd && <Markdown inlineLinks={primitiveReferences.map((reference) => ({ href: reference.href, label: reference.title, className: 'primitive-reference' }))}>{operation.content.bodyMd}</Markdown>}
    {operation.content.aliases.length > 0 && <div className="tag-list">{operation.content.aliases.map((alias) => <span key={alias}>{alias}</span>)}</div>}
    {operation.content.steps.length > 0 && <div className="bdd-steps">{operation.content.steps.map((step, stepIndex) => <div key={step.id || step.stableId || stepIndex}><strong>{step.keyword.toUpperCase()}</strong><span>{step.text}</span></div>)}</div>}
  </article>
}

function resolvePrimitiveReference(reference: AssistantChangeOperation['content']['references'][number], operations: AssistantChangeOperation[], pages: Page[]): { href: string; title: string } | null {
  if (reference.targetClientKey) {
    const operation = operations.find((candidate) => candidate.clientKey === reference.targetClientKey && candidate.kind === 'primitive')
    if (!operation) return null
    return { href: `#${draftOperationAnchor(reference.targetClientKey)}`, title: reference.targetTitle || operation.content.title }
  }
  if (reference.targetPageId) {
    const page = pages.find((candidate) => candidate.id === reference.targetPageId && candidate.kind === 'primitive')
    if (!page) return null
    return { href: `/page/${page.id}`, title: reference.targetTitle || page.title }
  }
  return null
}

function draftOperationAnchor(clientKey: string): string {
  return `draft-operation-${clientKey}`
}

function acceptedRevisionCount(count: number): string {
  if (count === 1) return '1 prijatú revíziu'
  if (count >= 2 && count <= 4) return `${count} prijaté revízie`
  return `${count} prijatých revízií`
}
