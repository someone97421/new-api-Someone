/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Check, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import { formatTaskLogBody } from '../../lib/task-details'
import type { TaskLog, TaskLogProperties } from '../../types'

interface TaskDetailsDialogProps {
  log: TaskLog
  open: boolean
  onOpenChange: (open: boolean) => void
}

function parseProperties(value: TaskLog['properties']): TaskLogProperties {
  if (!value) return {}
  if (typeof value === 'object') return value
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object'
      ? (parsed as TaskLogProperties)
      : {}
  } catch {
    return {}
  }
}

function BodyBlock(props: { label: string; emptyLabel: string; body: string }) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const displayBody = formatTaskLogBody(props.body)

  return (
    <section className='min-w-0 space-y-2'>
      <Label className='text-sm font-semibold'>{props.label}</Label>
      <div className='bg-muted/30 relative min-w-0 overflow-hidden rounded-md border'>
        {displayBody && (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            className='absolute top-1.5 right-1.5 z-10 size-7 p-0'
            onClick={() => copyToClipboard(displayBody)}
            title={t('Copy to clipboard')}
            aria-label={t('Copy to clipboard')}
          >
            {copiedText === displayBody ? (
              <Check className='size-3.5 text-green-600' />
            ) : (
              <Copy className='size-3.5' />
            )}
          </Button>
        )}
        {displayBody ? (
          <pre className='max-h-72 overflow-auto p-3 pr-11 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'>
            {displayBody}
          </pre>
        ) : (
          <p className='text-muted-foreground p-3 text-xs'>
            {props.emptyLabel}
          </p>
        )}
      </div>
    </section>
  )
}

export function TaskDetailsDialog(props: TaskDetailsDialogProps) {
  const { t } = useTranslation()
  const properties = parseProperties(props.log.properties)
  const requestBody = properties.request_body || ''
  let responseBody = properties.response_body || ''
  if (!responseBody && typeof props.log.data === 'string') {
    responseBody = props.log.data
  } else if (!responseBody && props.log.data) {
    responseBody = JSON.stringify(props.log.data)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Task Details')}
      description={t(
        'View the sanitized request and response bodies recorded for this asynchronous task.'
      )}
      contentClassName='sm:max-w-3xl'
      contentHeight='min(72vh, 760px)'
      bodyClassName='space-y-5'
    >
      <div className='grid gap-2 rounded-md border p-3 text-xs sm:grid-cols-2'>
        <div>
          <span className='text-muted-foreground'>{t('Task ID')}: </span>
          <span className='font-mono'>{props.log.task_id}</span>
        </div>
        <div>
          <span className='text-muted-foreground'>{t('Status')}: </span>
          <span>{props.log.status}</span>
        </div>
        <div>
          <span className='text-muted-foreground'>{t('Platform')}: </span>
          <span>{props.log.platform}</span>
        </div>
        <div>
          <span className='text-muted-foreground'>{t('Model')}: </span>
          <span className='font-mono'>
            {properties.origin_model_name ||
              properties.upstream_model_name ||
              '-'}
          </span>
        </div>
      </div>

      <p className='text-muted-foreground text-xs'>
        {t('Sensitive values are masked before storage.')}
      </p>

      <BodyBlock
        label={t('Request Body')}
        emptyLabel={t('No request body was recorded.')}
        body={requestBody}
      />
      <BodyBlock
        label={t('Response Body')}
        emptyLabel={t('No response body was recorded.')}
        body={responseBody}
      />
    </Dialog>
  )
}
