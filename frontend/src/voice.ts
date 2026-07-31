import { useCallback, useEffect, useRef, useState } from 'react'

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
  if (typeof window === 'undefined') return null
  const speechWindow = window as SpeechWindow
  return speechWindow.SpeechRecognition ?? speechWindow.webkitSpeechRecognition ?? null
}

function voiceErrorMessage(code: string): string {
  switch (code) {
  case 'not-allowed':
  case 'service-not-allowed':
    return 'Povoľte prístup k mikrofónu a skúste to znova.'
  case 'audio-capture':
    return 'Mikrofón nie je dostupný.'
  case 'no-speech':
    return 'Nezachytil som žiadnu reč. Skúste to znova.'
  default:
    return 'Hlasový vstup sa nepodarilo rozpoznať.'
  }
}

export function useSlovakVoiceInput(value: string, onChange: (value: string) => void, disabled: boolean) {
  const recognition = useRef<SpeechRecognitionLike | null>(null)
  const [listening, setListening] = useState(false)
  const [error, setError] = useState('')
  const supported = recognitionConstructor() !== null

  const stop = useCallback(() => {
    recognition.current?.stop()
  }, [])

  const start = useCallback(() => {
    if (disabled || recognition.current) return
    const Constructor = recognitionConstructor()
    if (!Constructor) {
      setError('Tento prehliadač nepodporuje hlasový vstup.')
      return
    }

    const instance = new Constructor()
    const original = value.trimEnd()
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
      setError(voiceErrorMessage(event.error))
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
      setError('Hlasový vstup sa nepodarilo spustiť.')
    }
  }, [disabled, onChange, value])

  useEffect(() => {
    if (disabled && recognition.current) recognition.current.abort()
  }, [disabled])

  useEffect(() => () => {
    recognition.current?.abort()
    recognition.current = null
  }, [])

  return { supported, listening, error, start, stop }
}
