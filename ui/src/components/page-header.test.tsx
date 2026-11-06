import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { PageHeader } from './page-header'

describe('PageHeader', () => {
  it('renders the shared title hierarchy with description and actions', () => {
    render(
      <PageHeader
        title="Pods"
        description="Running workloads"
        actions={<button type="button">Refresh</button>}
      />
    )

    const title = screen.getByRole('heading', { level: 1, name: 'Pods' })
    expect(title).toHaveClass('text-2xl', 'font-semibold')
    expect(screen.getByText('Running workloads')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument()
  })

  it('accepts action children for composed headers', () => {
    render(
      <PageHeader title="Node">
        <button type="button">Drain</button>
      </PageHeader>
    )

    expect(screen.getByRole('button', { name: 'Drain' })).toBeInTheDocument()
  })
})
