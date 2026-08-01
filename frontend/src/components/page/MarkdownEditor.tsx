import { Bold, Heading2, Italic, List } from 'lucide-react'
import { useRef } from 'react'
import { useI18n } from '../../i18n'

export function MarkdownEditor({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const { t } = useI18n()
  const ref = useRef<HTMLTextAreaElement>(null)
  const wrap = (before: string, after = before) => {
    const element = ref.current!
    const start = element.selectionStart
    const end = element.selectionEnd
    const selected = value.slice(start, end) || 'text'
    const next = value.slice(0, start) + before + selected + after + value.slice(end)
    onChange(next)
    requestAnimationFrame(() => { element.focus(); element.setSelectionRange(start + before.length, start + before.length + selected.length) })
  }
  return <div className="markdown-editor">
    <div className="editor-toolbar" aria-label={t('markdown.toolbar')}>
      <button type="button" onClick={() => wrap('**')} aria-label={t('markdown.bold')}><Bold size={15} /></button>
      <button type="button" onClick={() => wrap('_')} aria-label={t('markdown.italic')}><Italic size={15} /></button>
      <button type="button" onClick={() => wrap('## ', '')} aria-label={t('markdown.heading')}><Heading2 size={15} /></button>
      <button type="button" onClick={() => wrap('- ', '')} aria-label={t('markdown.list')}><List size={15} /></button>
      <span>Markdown</span>
    </div>
    <textarea ref={ref} value={value} onChange={(event) => onChange(event.target.value)} rows={13} aria-label={t('markdown.content')} placeholder={t('markdown.placeholder')} />
  </div>
}
