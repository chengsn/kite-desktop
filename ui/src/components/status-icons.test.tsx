import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { DeploymentStatusIcon } from './deployment-status-icon'
import { NodeStatusIcon } from './node-status-icon'
import { PodStatusIcon } from './pod-status-icon'

function iconClass(container: HTMLElement) {
  return container.querySelector('svg')?.getAttribute('class') ?? ''
}

describe('status icons use semantic theme tokens', () => {
  it('maps pod success, warning and error states to semantic colors', () => {
    const running = render(<PodStatusIcon status="Running" />)
    expect(iconClass(running.container)).toContain('fill-success')

    const warning = render(<PodStatusIcon status="NotReady" />)
    expect(iconClass(warning.container)).toContain('text-warning')

    const error = render(<PodStatusIcon status="CrashLoopBackOff" />)
    expect(iconClass(error.container)).toContain('fill-destructive')
  })

  it('maps node and deployment states to semantic colors', () => {
    const ready = render(<NodeStatusIcon status="Ready" />)
    expect(iconClass(ready.container)).toContain('fill-success')

    const notReady = render(<NodeStatusIcon status="NotReady" />)
    expect(iconClass(notReady.container)).toContain('fill-destructive')

    const available = render(<DeploymentStatusIcon status="Available" />)
    expect(iconClass(available.container)).toContain('fill-success')

    const unavailable = render(<DeploymentStatusIcon status="Not Available" />)
    expect(iconClass(unavailable.container)).toContain('fill-destructive')
  })
})
