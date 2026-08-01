import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, it, vi } from 'vitest'
import { MarkdownEditor } from './MarkdownEditor'

function Harness({ initial = 'hello world' }: { initial?: string }) {
  const [value, setValue] = useState(initial)
  return <MarkdownEditor value={value} onChange={setValue} />
}

it('edits text and applies every markdown toolbar format to selections and empty cursors', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { callback(0); return 1 })
  render(<Harness />)
  const textarea = screen.getByRole('textbox', { name: 'Obsah stránky' }) as HTMLTextAreaElement

  textarea.setSelectionRange(0, 5)
  await user.click(screen.getByRole('button', { name: 'Tučné' }))
  expect(textarea).toHaveValue('**hello** world')

  textarea.setSelectionRange(2, 7)
  await user.click(screen.getByRole('button', { name: 'Kurzíva' }))
  expect(textarea.value).toContain('_hello_')

  textarea.setSelectionRange(0, 0)
  await user.click(screen.getByRole('button', { name: 'Nadpis' }))
  expect(textarea.value).toContain('## text')

  textarea.setSelectionRange(textarea.value.length, textarea.value.length)
  await user.click(screen.getByRole('button', { name: 'Zoznam' }))
  expect(textarea.value).toContain('- text')

  await user.clear(textarea)
  await user.type(textarea, 'direct edit')
  expect(textarea).toHaveValue('direct edit')
  vi.unstubAllGlobals()
})
