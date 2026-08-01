import { beforeEach, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ render: vi.fn(), createRoot: vi.fn() }))
vi.mock('react-dom/client', () => ({ createRoot: mocks.createRoot }))
vi.mock('./App', () => ({ App: () => null }))

beforeEach(() => {
  vi.resetModules()
  mocks.render.mockReset()
  mocks.createRoot.mockReset().mockReturnValue({ render: mocks.render })
  document.body.innerHTML = '<div id="root"></div>'
})

it('mounts the application into the root element', async () => {
  await import('./main')
  expect(mocks.createRoot).toHaveBeenCalledWith(document.getElementById('root'))
  expect(mocks.render).toHaveBeenCalledOnce()
})
