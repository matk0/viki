import { Check, ChevronDown } from 'lucide-react'
import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'

export interface VikiSelectOption {
  value: string
  label: string
  disabled?: boolean
}

interface VikiSelectProps {
  ariaLabel: string
  listboxLabel?: string
  value: string
  options: readonly VikiSelectOption[]
  onChange: (value: string) => void
  disabled?: boolean
  compact?: boolean
  className?: string
  leadingIcon?: ReactNode
}

export function VikiSelect({
  ariaLabel,
  listboxLabel = ariaLabel,
  value,
  options,
  onChange,
  disabled = false,
  compact = false,
  className = '',
  leadingIcon,
}: VikiSelectProps) {
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([])
  const listboxId = useId()
  const matchingIndex = options.findIndex((option) => option.value === value)
  const selectedIndex = Math.max(0, matchingIndex)
  const selected = matchingIndex >= 0 ? options[matchingIndex] : undefined

  useEffect(() => {
    if (disabled) setOpen(false)
  }, [disabled])

  useEffect(() => {
    if (!open) return
    const closeOutside = (event: PointerEvent) => {
      if (event.target instanceof Node && !root.current?.contains(event.target)) setOpen(false)
    }
    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setOpen(false)
      trigger.current?.focus()
    }
    document.addEventListener('pointerdown', closeOutside)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeOutside)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    optionRefs.current[selectedIndex]?.focus()
  }, [open, selectedIndex])

  const openAt = (index: number) => {
    setOpen(true)
    requestAnimationFrame(() => optionRefs.current[index]?.focus())
  }

  const moveFocus = (current: number, delta: number) => {
    if (options.length === 0) return
    let next = current
    do next = (next + delta + options.length) % options.length
    while (options[next]?.disabled && next !== current)
    optionRefs.current[next]?.focus()
  }

  const choose = (option: VikiSelectOption) => {
    if (option.disabled) return
    if (option.value !== value) onChange(option.value)
    setOpen(false)
    trigger.current?.focus()
  }

  const handleTriggerKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      const direction = event.key === 'ArrowDown' ? 1 : -1
      const target = open ? selectedIndex : Math.max(0, selectedIndex + direction)
      openAt(Math.min(options.length - 1, target))
    }
  }

  const handleOptionKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number, option: VikiSelectOption) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      moveFocus(index, event.key === 'ArrowDown' ? 1 : -1)
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault()
      optionRefs.current[event.key === 'Home' ? 0 : options.length - 1]?.focus()
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      choose(option)
    } else if (event.key === 'Tab') {
      setOpen(false)
    }
  }

  const classes = ['viki-select', compact ? 'compact' : '', open ? 'open' : '', leadingIcon ? 'has-leading' : '', className].filter(Boolean).join(' ')

  return <div className={classes} ref={root}>
    <button
      ref={trigger}
      type="button"
      className="viki-select-trigger"
      aria-label={ariaLabel}
      aria-haspopup="listbox"
      aria-expanded={open}
      aria-controls={listboxId}
      disabled={disabled}
      onClick={() => setOpen((current) => !current)}
      onKeyDown={handleTriggerKeyDown}
    >
      {leadingIcon && <span className="viki-select-leading" aria-hidden="true">{leadingIcon}</span>}
      <span className="viki-select-value">{selected?.label ?? ''}</span>
      <ChevronDown className="viki-select-chevron" size={compact ? 14 : 16} />
    </button>
    {open && <div id={listboxId} className="viki-select-menu" role="listbox" aria-label={listboxLabel}>
      {options.map((option, index) => {
        const isSelected = option.value === value
        return <button
          ref={(element) => { optionRefs.current[index] = element }}
          type="button"
          role="option"
          aria-selected={isSelected}
          disabled={option.disabled}
          className={isSelected ? 'selected' : ''}
          key={option.value}
          onClick={() => choose(option)}
          onKeyDown={(event) => handleOptionKeyDown(event, index, option)}
        >
          <span>{option.label}</span>
          {isSelected && <Check size={compact ? 13 : 15} />}
        </button>
      })}
    </div>}
  </div>
}
