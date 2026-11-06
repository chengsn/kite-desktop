import { useEffect, useState } from 'react'
import { IconClipboardText } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { ResourceType } from '@/types/api'
import { useDescribe } from '@/lib/api'
import { trackResourceAction } from '@/lib/analytics'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'

import { TextViewer } from './text-viewer'
import { Button } from './ui/button'

export function DescribeDialog({
  resourceType,
  namespace,
  name,
}: {
  resourceType: ResourceType
  namespace?: string
  name: string
}) {
  const [isDescribeOpen, setIsDescribeOpen] = useState(false)
  const { t } = useTranslation()
  const { data: describeText } = useDescribe(resourceType, name, namespace, {
    enabled: isDescribeOpen,
    staleTime: 0,
  })

  useEffect(() => {
    if (!isDescribeOpen) {
      return
    }

    trackResourceAction(resourceType, 'describe_open')
  }, [isDescribeOpen, resourceType])

  const describeTitle = `kubectl describe ${resourceType}${
    namespace ? ` -n ${namespace}` : ''
  } ${name}`

  return (
    <Dialog open={isDescribeOpen} onOpenChange={setIsDescribeOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <IconClipboardText className="w-4 h-4" />
          {t('describe.button')}
        </Button>
      </DialogTrigger>
      <DialogContent className="!max-w-dvw">
        <DialogHeader className="sr-only">
          <DialogTitle>{describeTitle}</DialogTitle>
          <DialogDescription>
            {t('describe.dialogDescription')}
          </DialogDescription>
        </DialogHeader>
        <TextViewer title={describeTitle} value={describeText?.result || ''} />
      </DialogContent>
    </Dialog>
  )
}
