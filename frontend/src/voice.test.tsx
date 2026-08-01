import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Translate } from './i18n'
import { useSlovakVoiceInput } from './voice'

const t = ((key: string) => key) as Translate

class Recognition {
  static latest: Recognition | null = null
  static throwOnStart = false

  lang = ''
  continuous = false
  interimResults = false
  onresult: ((event: { resultIndex: number; results: ArrayLike<{ readonly 0: { transcript: string } }> }) => void) | null = null
  onerror: ((event: { error: string }) => void) | null = null
  onend: (() => void) | null = null
  start = vi.fn(() => { if (Recognition.throwOnStart) throw new Error('start failed') })
  stop = vi.fn()
  abort = vi.fn()

  constructor() { Recognition.latest = this }
}

function installRecognition(value: typeof Recognition | undefined, webkit = false) {
  Object.defineProperty(window, 'SpeechRecognition', { configurable: true, value: webkit ? undefined : value })
  Object.defineProperty(window, 'webkitSpeechRecognition', { configurable: true, value: webkit ? value : undefined })
}

beforeEach(() => {
  Recognition.latest = null
  Recognition.throwOnStart = false
  installRecognition(Recognition)
})

describe('useSlovakVoiceInput', () => {
  it('reports unsupported browsers and refuses to start', () => {
    installRecognition(undefined)
    const onChange = vi.fn()
    const { result } = renderHook(() => useSlovakVoiceInput('', onChange, false, t))

    expect(result.current.supported).toBe(false)
    act(() => result.current.start())
    expect(result.current.error).toBe('voice.unsupported')
    expect(onChange).not.toHaveBeenCalled()
  })

  it('uses the WebKit fallback, appends Slovak transcripts, stops, and restores the original value on cancel', () => {
    installRecognition(Recognition, true)
    const onChange = vi.fn()
    const { result } = renderHook(() => useSlovakVoiceInput('Existing  ', onChange, false, t))

    act(() => result.current.start())
    const instance = Recognition.latest!
    expect(result.current.supported).toBe(true)
    expect(result.current.listening).toBe(true)
    expect(instance).toMatchObject({ lang: 'sk-SK', continuous: true, interimResults: true })
    expect(instance.start).toHaveBeenCalledOnce()

    act(() => instance.onresult?.({ resultIndex: 0, results: [{ 0: { transcript: '  first ' } }, { 0: { transcript: 'second' } }] }))
    expect(onChange).toHaveBeenLastCalledWith('Existing first second')
    act(() => result.current.stop())
    expect(instance.stop).toHaveBeenCalledOnce()
    act(() => result.current.cancel())
    expect(instance.abort).toHaveBeenCalledOnce()
    expect(onChange).toHaveBeenLastCalledWith('Existing  ')
    expect(result.current.listening).toBe(false)
    act(() => instance.onend?.())
    act(() => result.current.cancel())
    expect(instance.abort).toHaveBeenCalledOnce()
  })

  it('handles empty and missing transcript fragments and finishes naturally', () => {
    const onChange = vi.fn()
    const { result } = renderHook(() => useSlovakVoiceInput('', onChange, false, t))
    act(() => result.current.start())
    const instance = Recognition.latest!

    act(() => instance.onresult?.({ resultIndex: 0, results: [{ 0: { transcript: 'spoken' } }, {} as { 0: { transcript: string } }] }))
    expect(onChange).toHaveBeenLastCalledWith('spoken')
    act(() => instance.onend?.())
    expect(result.current.listening).toBe(false)

    act(() => result.current.start())
    expect(Recognition.latest).not.toBe(instance)
  })

  it.each([
    ['not-allowed', 'voice.notAllowed'],
    ['service-not-allowed', 'voice.notAllowed'],
    ['audio-capture', 'voice.unavailable'],
    ['no-speech', 'voice.noSpeech'],
    ['network', 'voice.unrecognized'],
  ])('maps recognition error %s', (code, expected) => {
    const { result } = renderHook(() => useSlovakVoiceInput('', vi.fn(), false, t))
    act(() => result.current.start())
    act(() => Recognition.latest?.onerror?.({ error: code }))
    expect(result.current.error).toBe(expected)
  })

  it('recovers from synchronous start failures', () => {
    Recognition.throwOnStart = true
    const { result } = renderHook(() => useSlovakVoiceInput('', vi.fn(), false, t))
    act(() => result.current.start())

    expect(result.current.listening).toBe(false)
    expect(result.current.error).toBe('voice.startFailed')
  })

  it('does not start while disabled or already listening', () => {
    const { result } = renderHook(() => useSlovakVoiceInput('', vi.fn(), false, t))
    act(() => result.current.start())
    const instance = Recognition.latest!
    act(() => result.current.start())
    expect(instance.start).toHaveBeenCalledOnce()

    const disabled = renderHook(() => useSlovakVoiceInput('', vi.fn(), true, t))
    act(() => disabled.result.current.start())
    expect(Recognition.latest).toBe(instance)
  })

  it('aborts active recognition when disabled and on unmount', () => {
    const onChange = vi.fn()
    const view = renderHook(({ disabled }) => useSlovakVoiceInput('', onChange, disabled, t), { initialProps: { disabled: false } })
    act(() => view.result.current.start())
    const first = Recognition.latest!
    view.rerender({ disabled: true })
    expect(first.abort).toHaveBeenCalledOnce()

    view.rerender({ disabled: false })
    act(() => first.onend?.())
    act(() => view.result.current.start())
    const second = Recognition.latest!
    view.unmount()
    expect(second.abort).toHaveBeenCalledOnce()
  })
})
