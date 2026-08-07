import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

const focusableSelector = 'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function focusableElements(container: HTMLElement) {
  return [...container.querySelectorAll<HTMLElement>(focusableSelector)]
}

export function Modal({ children, className, onClose }: { children: ReactNode; className?: string; onClose: () => void }) {
  const backdropRef = useRef<HTMLDivElement>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement
    const backdrop = backdropRef.current!
    document.body.classList.add('modal-open')
    if (!backdrop.contains(document.activeElement)) focusableElements(backdrop)[0]?.focus()
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        event.stopPropagation()
        onCloseRef.current()
        return
      }
      if (event.key !== 'Tab') return
      const focusable = focusableElements(backdrop)
      if (focusable.length === 0) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const movingBackward = event.shiftKey && document.activeElement === first
      const movingForward = !event.shiftKey && document.activeElement === last
      if (!movingBackward && !movingForward) return
      event.preventDefault()
      if (movingBackward) last.focus()
      else first.focus()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('keydown', closeOnEscape)
      document.body.classList.remove('modal-open')
      previouslyFocused.focus()
    }
  }, [])

  return createPortal(
    <div
      className={`modal-backdrop${className ? ` ${className}` : ''}`}
      ref={backdropRef}
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      {children}
    </div>,
    document.body,
  )
}
