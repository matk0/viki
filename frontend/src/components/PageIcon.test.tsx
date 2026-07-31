import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PageIcon } from './PageIcon'

describe('PageIcon', () => {
  it('uses the same icons as the primary navigation', () => {
    const primitive = render(<PageIcon page={{ kind: 'primitive', primitiveKind: 'noun' }} />)
    expect(primitive.container.querySelector('.lucide-box')).not.toBeNull()
    primitive.unmount()

    const scenario = render(<PageIcon page={{ kind: 'scenario' }} />)
    expect(scenario.container.querySelector('.lucide-workflow')).not.toBeNull()
  })
})
