import '@/i18n'

import { createColumnHelper } from '@tanstack/react-table'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import i18n from 'i18next'
import { Deployment } from 'kubernetes-types/apps/v1'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ResourceTable } from './resource-table'

const deleteResourceMock = vi.fn()

const demoDeployment = {
  metadata: {
    name: 'demo',
    namespace: 'default',
    uid: 'deploy-1',
  },
}
let resourcesData = [demoDeployment]
let resourcesError: Error | null = null
const refetchMock = vi.fn()

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')

  return {
    ...actual,
    deleteResource: (...args: unknown[]) => deleteResourceMock(...args),
    useResources: () => ({
      isLoading: false,
      data: resourcesData,
      isError: Boolean(resourcesError),
      error: resourcesError,
      refetch: refetchMock,
    }),
    useResourcesWatch: () => ({
      data: undefined,
      isLoading: false,
      error: null,
      isConnected: false,
      refetch: vi.fn(),
    }),
  }
})

describe('ResourceTable batch delete confirmation', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
    ;(
      globalThis as typeof globalThis & {
        ResizeObserver: new (callback: ResizeObserverCallback) => ResizeObserver
      }
    ).ResizeObserver = class {
      observe() {}
      disconnect() {}
      unobserve() {}
    } as unknown as new (callback: ResizeObserverCallback) => ResizeObserver
    localStorage.setItem('current-cluster', 'test-cluster')
    resourcesData = [demoDeployment]
    resourcesError = null
    refetchMock.mockReset()
    deleteResourceMock.mockReset()
    deleteResourceMock.mockResolvedValue(undefined)
  })

  it('keeps the table header and lets the empty state size the scroller', async () => {
    await i18n.changeLanguage('en')
    resourcesData = []

    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.name', {
            header: 'Name',
            cell: ({ row }) => row.original.metadata?.name,
          }),
        ]}
      />
    )

    expect(
      screen.getByRole('columnheader', { name: 'Name' })
    ).toBeInTheDocument()
    const emptyState = screen.getByText('No resources found')
    expect(emptyState).toBeInTheDocument()

    const scrollContainer = emptyState.closest('.overflow-auto')
    expect(scrollContainer).not.toBeNull()

    await act(async () => {
      await new Promise((resolve) => {
        window.requestAnimationFrame(() => resolve(null))
      })
    })

    expect(scrollContainer).not.toHaveClass('min-h-[240px]')
    expect(scrollContainer?.getAttribute('style') ?? '').not.toContain('height')
  })

  it('keeps loaded rows visible and offers a retry when a refresh fails', () => {
    resourcesError = new Error('connection reset')
    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.name', {
            header: 'Name',
            cell: ({ row }) => row.original.metadata?.name,
          }),
        ]}
      />
    )

    expect(screen.getByText('demo')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent(
      "Couldn't refresh Deployments"
    )

    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(refetchMock).toHaveBeenCalledTimes(1)
  })

  it('shows the full error state when the initial load fails', () => {
    resourcesData = []
    resourcesError = new Error('connection reset')
    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.name', {
            header: 'Name',
            cell: ({ row }) => row.original.metadata?.name,
          }),
        ]}
      />
    )

    expect(
      screen.getByRole('heading', { name: 'Error loading deployments' })
    ).toBeInTheDocument()
    expect(
      screen.queryByText("Couldn't refresh Deployments")
    ).not.toBeInTheDocument()
  })

  it('marks sortable headers with aria-sort', () => {
    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.name', {
            header: 'Name',
            cell: ({ row }) => row.original.metadata?.name,
          }),
        ]}
      />
    )

    const header = screen.getByRole('columnheader', { name: 'Name' })
    expect(header).toHaveAttribute('aria-sort', 'none')

    fireEvent.click(screen.getByRole('button', { name: 'Name' }))
    expect(header).toHaveAttribute('aria-sort', 'ascending')
  })

  it('consolidates column filters into a single Filters popover', async () => {
    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.namespace', {
            header: 'Namespace',
            enableColumnFilter: true,
            cell: ({ row }) => row.original.metadata?.namespace,
          }),
        ]}
      />
    )

    expect(screen.queryByText('Filter Namespace')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Filters' }))

    expect(
      await screen.findByRole('heading', { name: 'Namespace' })
    ).toBeInTheDocument()
  })

  it('keeps the Filters control available when the result set is empty', () => {
    resourcesData = []
    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.namespace', {
            header: 'Namespace',
            enableColumnFilter: true,
            cell: ({ row }) => row.original.metadata?.namespace,
          }),
        ]}
      />
    )

    expect(screen.getByRole('button', { name: 'Filters' })).toBeInTheDocument()
  })

  it('requires the global confirmation keyword before deleting selected rows', async () => {
    await i18n.changeLanguage('zh')

    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.name', {
            header: 'Name',
            cell: ({ row }) => row.original.metadata?.name,
          }),
        ]}
      />
    )

    fireEvent.click(screen.getByLabelText('选择行'))
    fireEvent.click(screen.getByRole('button', { name: '删除 (1)' }))

    const confirmButton = screen.getByRole('button', { name: '删除' })
    expect(confirmButton).toBeDisabled()

    fireEvent.change(screen.getByLabelText(/输入/), {
      target: { value: 'demo' },
    })
    expect(confirmButton).toBeDisabled()

    fireEvent.change(screen.getByLabelText(/输入/), {
      target: { value: '确认删除' },
    })
    expect(confirmButton).not.toBeDisabled()

    fireEvent.click(confirmButton)

    await waitFor(() => {
      expect(deleteResourceMock).toHaveBeenCalledWith(
        'deployments',
        'demo',
        undefined
      )
    })
  })

  it('opens a row context menu and triggers the selected action', async () => {
    const onInspect = vi.fn()
    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.name', {
            header: 'Name',
            cell: ({ row }) => row.original.metadata?.name,
          }),
        ]}
        getRowContextMenuItems={(item) => [
          {
            key: 'inspect',
            label: 'Inspect resource',
            onSelect: () => onInspect(item.metadata?.name),
          },
        ]}
      />
    )

    fireEvent.contextMenu(screen.getByText('demo'))

    const menuItem = await screen.findByText('Inspect resource')
    fireEvent.click(menuItem)

    expect(onInspect).toHaveBeenCalledWith('demo')
  })

  it('renders an actions column menu that uses the same row actions', async () => {
    const onInspect = vi.fn()
    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.name', {
            header: 'Name',
            cell: ({ row }) => row.original.metadata?.name,
          }),
        ]}
        getRowContextMenuItems={(item) => [
          {
            key: 'inspect',
            label: 'Inspect resource',
            onSelect: () => onInspect(item.metadata?.name),
          },
        ]}
      />
    )

    fireEvent.pointerEnter(
      screen.getAllByRole('button', { name: /Actions|操作/i })[0]
    )

    const menuItem = await screen.findByText('Inspect resource')
    fireEvent.click(menuItem)

    expect(onInspect).toHaveBeenCalledWith('demo')
  })

  it('opens the actions menu on hover and closes after leaving trigger and menu', async () => {
    const columnHelper = createColumnHelper<Deployment>()

    render(
      <ResourceTable
        resourceName="Deployments"
        resourceType="deployments"
        clusterScope={true}
        columns={[
          columnHelper.accessor('metadata.name', {
            header: 'Name',
            cell: ({ row }) => row.original.metadata?.name,
          }),
        ]}
        getRowContextMenuItems={() => [
          {
            key: 'inspect',
            label: 'Inspect resource',
            onSelect: vi.fn(),
          },
        ]}
      />
    )

    const trigger = screen.getAllByRole('button', { name: /Actions|操作/i })[0]
    fireEvent.pointerEnter(trigger)

    const menuItem = await screen.findByText('Inspect resource')
    expect(menuItem).toBeInTheDocument()

    fireEvent.pointerLeave(trigger)
    fireEvent.pointerLeave(menuItem)

    await waitFor(() => {
      expect(screen.queryByText('Inspect resource')).not.toBeInTheDocument()
    })
  })
})
