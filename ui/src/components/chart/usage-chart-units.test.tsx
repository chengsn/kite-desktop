import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { UsageDataPoint } from '@/types/api'

import CPUUsageChart from './cpu-usage-chart'
import MemoryUsageChart from './memory-usage-chart'

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>()

  return {
    ...actual,
    useTranslation: () => ({
      t: (key: string) => key,
    }),
  }
})

const data: UsageDataPoint[] = [
  { timestamp: '2026-03-27T11:00:00Z', value: 42 },
  { timestamp: '2026-03-27T11:05:00Z', value: 55 },
]

describe('CPUUsageChart units', () => {
  it('labels the chart as a utilization when the backend reports percent', () => {
    render(<CPUUsageChart data={data} unit="percent" />)

    expect(screen.getByText('monitoring.cpuUsage')).toBeInTheDocument()
  })

  it('labels the chart as absolute cores when the pod declares no limit or request', () => {
    render(<CPUUsageChart data={data} unit="cores" />)

    expect(screen.getByText('monitoring.cpuUsageAbsolute')).toBeInTheDocument()
  })

  it('keeps the neutral title while the backend has not reported a unit', () => {
    render(<CPUUsageChart data={data} />)

    expect(screen.getByText('monitoring.cpuUsage')).toBeInTheDocument()
    expect(
      screen.queryByText('monitoring.cpuUsageAbsolute')
    ).not.toBeInTheDocument()
  })
})

describe('MemoryUsageChart units', () => {
  it('labels the chart as a utilization when the backend reports percent', () => {
    render(<MemoryUsageChart data={data} unit="percent" />)

    expect(screen.getByText('monitoring.memoryUsage')).toBeInTheDocument()
  })

  it('labels the chart as absolute megabytes when the pod declares no limit or request', () => {
    render(<MemoryUsageChart data={data} unit="mb" />)

    expect(
      screen.getByText('monitoring.memoryUsageAbsolute')
    ).toBeInTheDocument()
  })

  it('keeps the neutral title while the backend has not reported a unit', () => {
    render(<MemoryUsageChart data={data} />)

    expect(screen.getByText('monitoring.memoryUsage')).toBeInTheDocument()
    expect(
      screen.queryByText('monitoring.memoryUsageAbsolute')
    ).not.toBeInTheDocument()
  })
})
