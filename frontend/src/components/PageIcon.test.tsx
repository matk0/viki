import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PageIcon } from './PageIcon'

describe('PageIcon', () => {
  it('uses the same icons as the primary navigation', () => {
    const concept = render(<PageIcon page={{ kind: 'concept', conceptKind: 'noun' }} />)
    expect(concept.container.querySelector('.lucide-box')).not.toBeNull()
    concept.unmount()

    const feature = render(<PageIcon page={{ kind: 'feature' }} />)
    expect(feature.container.querySelector('.lucide-workflow')).not.toBeNull()
    feature.unmount()

    const scenario = render(<PageIcon page={{ kind: 'scenario' }} />)
    expect(scenario.container.querySelector('.lucide-file-check-corner')).not.toBeNull()
  })
})
