import { useCallback, useEffect, useRef, useState } from 'react'
import type { Translate } from './i18n'

interface SpeechRecognitionResultLike {
  readonly 0: { readonly transcript: string }
}

interface SpeechRecognitionEventLike {
  readonly resultIndex: number
  readonly results: ArrayLike<SpeechRecognitionResultLike>
}

interface SpeechRecognitionErrorLike {
  readonly error: string
}

interface SpeechRecognitionLike {
  lang: string
  continuous: boolean
  interimResults: boolean
  onresult: ((event: SpeechRecognitionEventLike) => void) | null
  onerror: ((event: SpeechRecognitionErrorLike) => void) | null
  onend: (() => void) | null
  start(): void
  stop(): void
  abort(): void
}

type SpeechRecognitionConstructor = new () => SpeechRecognitionLike

type SpeechWindow = Window & {
  SpeechRecognition?: SpeechRecognitionConstructor
  webkitSpeechRecognition?: SpeechRecognitionConstructor
}

function recognitionConstructor(): SpeechRecognitionConstructor | null {
  const speechWindow = window as SpeechWindow
  return speechWindow.SpeechRecognition ?? speechWindow.webkitSpeechRecognition ?? null
}

function voiceErrorMessage(code: string, t: Translate): string {
  switch (code) {
  case 'not-allowed':
  case 'service-not-allowed':
    return t('voice.notAllowed')
  case 'audio-capture':
    return t('voice.unavailable')
  case 'no-speech':
    return t('voice.noSpeech')
  default:
    return t('voice.unrecognized')
  }
}

export function useSlovakVoiceInput(value: string, onChange: (value: string) => void, disabled: boolean, t: Translate) {
  const recognition = useRef<SpeechRecognitionLike | null>(null)
  const initialValue = useRef('')
  const [listening, setListening] = useState(false)
  const [error, setError] = useState('')
  const supported = recognitionConstructor() !== null

  const stop = useCallback(() => {
    recognition.current?.stop()
  }, [])

  const cancel = useCallback(() => {
    const instance = recognition.current
    if (!instance) return
    recognition.current = null
    instance.abort()
    onChange(initialValue.current)
    setListening(false)
  }, [onChange])

  const start = useCallback(() => {
    if (disabled || recognition.current) return
    const Constructor = recognitionConstructor()
    if (!Constructor) {
      setError(t('voice.unsupported'))
      return
    }

    const instance = new Constructor()
    const original = value.trimEnd()
    initialValue.current = value
    instance.lang = 'sk-SK'
    instance.continuous = true
    instance.interimResults = true
    instance.onresult = (event) => {
      let transcript = ''
      for (let index = 0; index < event.results.length; index += 1) {
        transcript += event.results[index]?.[0]?.transcript ?? ''
      }
      const spoken = transcript.trimStart()
      onChange(original && spoken ? `${original} ${spoken}` : original || spoken)
    }
    instance.onerror = (event) => {
      setError(voiceErrorMessage(event.error, t))
    }
    instance.onend = () => {
      if (recognition.current === instance) recognition.current = null
      setListening(false)
    }
    recognition.current = instance
    setError('')
    setListening(true)
    try {
      instance.start()
    } catch {
      recognition.current = null
      setListening(false)
      setError(t('voice.startFailed'))
    }
  }, [disabled, onChange, t, value])

  useEffect(() => {
    if (disabled && recognition.current) recognition.current.abort()
  }, [disabled])

  useEffect(() => () => {
    recognition.current?.abort()
    recognition.current = null
  }, [])

  return { supported, listening, error, start, stop, cancel }
}
