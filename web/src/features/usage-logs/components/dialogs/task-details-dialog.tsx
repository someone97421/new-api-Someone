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
import { Shield01Icon, Wrench01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { DynamicPricingBreakdown } from '@/features/pricing/components/dynamic-pricing-breakdown'
import { formatLogQuota, formatTimestampToDate } from '@/lib/format'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { getTaskData } from '../../api'
import { taskActionMapper, taskStatusMapper } from '../../lib/mappers'
import { resolveTaskDetailAccess } from '../../lib/task-details'
import type { TaskLog } from '../../types'
import { PluginAuthorLink } from '../plugin-author-link'

function DetailRow(props: {
  label: React.ReactNode
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='grid min-w-0 grid-cols-[6rem_minmax(0,1fr)] gap-2 text-sm sm:grid-cols-[8rem_minmax(0,1fr)]'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span
        className={cn(
          'min-w-0 text-xs break-all sm:wrap-break-word',
          props.mono && 'font-mono'
        )}
      >
        {props.value}
      </span>
    </div>
  )
}

function DetailSection(props: {
  label: string
  icon?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className='min-w-0 space-y-1.5'>
      <Label className='flex items-center gap-1.5 text-xs font-semibold'>
        {props.icon}
        {props.label}
      </Label>
      <div className='bg-muted/30 min-w-0 space-y-1.5 rounded-md border p-2.5'>
        {props.children}
      </div>
    </section>
  )
}

function formatTaskTimestamp(value?: number): string {
  return value ? formatTimestampToDate(value, 'seconds') : '-'
}

interface TaskDetailsDialogProps {
  log: TaskLog
  isAdmin: boolean
  isRoot: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TaskDetailsDialog(props: TaskDetailsDialogProps) {
  return <TaskDetailsContent key={props.log.task_id} {...props} />
}

function TaskDetailsContent(props: TaskDetailsDialogProps) {
  const { t } = useTranslation()
  const [showTaskData, setShowTaskData] = useState(false)
  const [showBillingEstimate, setShowBillingEstimate] = useState(false)
  const canReadTaskData = props.isAdmin && props.log.result_discarded !== true
  const taskDataQuery = useQuery({
    queryKey: ['usage-logs', 'task-data', props.log.task_id],
    queryFn: async () => {
      const response = requireServerSuccess(
        await getTaskData(props.log.task_id)
      )
      if (!response.data || response.data.task_id !== props.log.task_id) {
        throw new Error(t('Invalid server response'))
      }
      return response.data
    },
    enabled:
      canReadTaskData && props.open && (showTaskData || showBillingEstimate),
    retry: false,
    meta: { errorToast: false },
  })
  const access = resolveTaskDetailAccess(props.log, props.isAdmin, props.isRoot)
  const plugin = access.plugin
  const runtime = access.runtime
  const properties = props.log.properties
  const billing = taskDataQuery.data?.billing
  const usageFacts: Record<string, string | number> = {}
  for (const [field, value] of Object.entries(billing?.usage_facts ?? {})) {
    if (
      typeof value === 'string' ||
      (typeof value === 'number' && Number.isFinite(value))
    ) {
      usageFacts[field] = value
    } else if (typeof value === 'boolean') {
      // The pricing display compares enum and boolean conditions as strings.
      usageFacts[field] = String(value)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          setShowTaskData(false)
          setShowBillingEstimate(false)
        }
        props.onOpenChange(open)
      }}
      title={
        <span className='flex items-center gap-2'>
          {t('Task Details')}
          <StatusBadge
            label={t(
              taskStatusMapper.getLabel(
                props.log.status,
                props.log.status || 'Submitting'
              )
            )}
            variant={taskStatusMapper.getVariant(props.log.status)}
            size='sm'
            copyable={false}
          />
        </span>
      }
      description={t('View the complete details for this task')}
      contentClassName='min-w-0 overflow-hidden sm:max-w-2xl'
      contentHeight='min(72dvh, 720px)'
      bodyClassName='pr-2 sm:pr-4'
    >
      <div className='space-y-3'>
        <DetailSection label={t('Basic Information')}>
          <DetailRow label={t('Task ID')} value={props.log.task_id} mono />
          <DetailRow label={t('Platform')} value={props.log.platform} mono />
          <DetailRow
            label={t('Action')}
            value={t(taskActionMapper.getLabel(props.log.action))}
          />
          <DetailRow
            label={t('Progress')}
            value={props.log.progress || '-'}
            mono
          />
          <DetailRow
            label={t('Submit Time')}
            value={formatTaskTimestamp(props.log.submit_time)}
            mono
          />
          <DetailRow
            label={t('Start Time')}
            value={formatTaskTimestamp(props.log.start_time)}
            mono
          />
          <DetailRow
            label={t('Finish Time')}
            value={formatTaskTimestamp(props.log.finish_time)}
            mono
          />
          {properties?.origin_model_name ? (
            <DetailRow
              label={t('Original Model')}
              value={properties.origin_model_name}
              mono
            />
          ) : null}
          {properties?.upstream_model_name ? (
            <DetailRow
              label={t('Actual Model')}
              value={properties.upstream_model_name}
              mono
            />
          ) : null}
          {props.log.fail_reason ? (
            <DetailRow label={t('Fail Reason')} value={props.log.fail_reason} />
          ) : null}
          {canReadTaskData ? (
            <div className='space-y-2 pt-1'>
              <div className='flex flex-wrap gap-2'>
                <Button
                  variant='link'
                  size='sm'
                  onClick={() => setShowTaskData((value) => !value)}
                  aria-expanded={showTaskData}
                >
                  {showTaskData ? t('Hide Task Data') : t('View Task Data')}
                </Button>
                <Button
                  variant='link'
                  size='sm'
                  onClick={() => setShowBillingEstimate((value) => !value)}
                  aria-expanded={showBillingEstimate}
                >
                  {showBillingEstimate
                    ? t('Hide Billing Estimate')
                    : t('View Billing Estimate')}
                </Button>
              </div>
              {showTaskData || showBillingEstimate ? (
                <>
                  {taskDataQuery.isPending ? (
                    <p className='text-xs'>{t('Loading...')}</p>
                  ) : null}
                  {taskDataQuery.isError ? (
                    <ErrorState
                      title={t('Failed to load task data')}
                      description={getServerErrorMessage(taskDataQuery.error)}
                      onRetry={() => void taskDataQuery.refetch()}
                      className='min-h-0 p-2'
                    />
                  ) : null}
                </>
              ) : null}
              {showTaskData && taskDataQuery.isSuccess ? (
                <pre className='bg-background max-h-64 overflow-auto rounded border p-2 text-[10px] whitespace-pre-wrap'>
                  {JSON.stringify(taskDataQuery.data.data, null, 2)}
                </pre>
              ) : null}
              {showBillingEstimate && taskDataQuery.isSuccess ? (
                <DetailSection label={t('Billing Estimate')}>
                  {billing?.estimated === true && billing.expression ? (
                    <>
                      <p className='text-muted-foreground text-xs'>
                        {t('This estimate is not the final bill.')}
                      </p>
                      <DetailRow
                        label={t('Tier')}
                        value={billing.tier || '-'}
                        mono
                      />
                      {billing.usage_schema &&
                      Object.keys(billing.usage_schema).length > 0 ? (
                        <DynamicPricingBreakdown
                          compact
                          billingExpr={billing.expression}
                          matchedTierLabel={billing.tier}
                          usageSchema={billing.usage_schema}
                          usageFacts={usageFacts}
                        />
                      ) : (
                        <>
                          <p className='text-muted-foreground text-xs'>
                            {t(
                              'Task usage metadata is unavailable. Pricing details cannot be displayed.'
                            )}
                          </p>
                          <DetailRow
                            label={t('Expression')}
                            value={billing.expression}
                            mono
                          />
                        </>
                      )}
                      <DetailRow
                        label={t('Usage facts')}
                        value={
                          <pre className='max-h-40 overflow-auto whitespace-pre-wrap'>
                            {JSON.stringify(usageFacts, null, 2)}
                          </pre>
                        }
                      />
                    </>
                  ) : (
                    <p className='text-muted-foreground text-xs'>
                      {t('No billing estimate is available.')}
                    </p>
                  )}
                </DetailSection>
              ) : null}
            </div>
          ) : null}
        </DetailSection>

        {props.isAdmin ? (
          <DetailSection
            label={t('Admin Only')}
            icon={
              <HugeiconsIcon
                icon={Shield01Icon}
                className='size-3.5 text-blue-500'
                strokeWidth={2}
              />
            }
          >
            <DetailRow
              label={t('User')}
              value={props.log.username || String(props.log.user_id)}
            />
            <DetailRow
              label={t('Channel')}
              value={`#${props.log.channel_id}`}
              mono
            />
            <DetailRow label={t('Group')} value={props.log.group || '-'} />
            <DetailRow
              label={t('Quota')}
              value={formatLogQuota(props.log.quota)}
              mono
            />
            {props.log.admin_info?.request_id ? (
              <DetailRow
                label={t('Request ID')}
                value={props.log.admin_info.request_id}
                mono
              />
            ) : null}
            {props.log.admin_info?.request_path ? (
              <DetailRow
                label={t('Request Path')}
                value={props.log.admin_info.request_path}
                mono
              />
            ) : null}
            {plugin ? (
              <>
                <DetailRow
                  label={t('Task Plugin')}
                  value={plugin.name || plugin.key}
                />
                <DetailRow label={t('Plugin key')} value={plugin.key} mono />
                <DetailRow
                  label={t('Version')}
                  value={plugin.version || '-'}
                  mono
                />
                {plugin.author ? (
                  <DetailRow
                    label={t('Plugin author')}
                    value={<PluginAuthorLink author={plugin.author} showUrl />}
                  />
                ) : null}
              </>
            ) : null}
          </DetailSection>
        ) : null}

        {props.isAdmin && props.isRoot && props.log.root_info ? (
          <DetailSection
            label={t('Root Diagnostics')}
            icon={
              <HugeiconsIcon
                icon={Wrench01Icon}
                className='size-3.5 text-amber-500'
                strokeWidth={2}
              />
            }
          >
            {runtime ? (
              <>
                <DetailRow
                  label={t('API Version')}
                  value={String(runtime.api_version)}
                  mono
                />
                <DetailRow
                  label={t('Plugin Generation')}
                  value={String(runtime.generation)}
                  mono
                />
              </>
            ) : null}
            {access.upstreamTaskId ? (
              <DetailRow
                label={t('Upstream Task ID')}
                value={access.upstreamTaskId}
                mono
              />
            ) : null}
            {access.nodeName ? (
              <DetailRow label={t('Node Name')} value={access.nodeName} mono />
            ) : null}
          </DetailSection>
        ) : null}
      </div>
    </Dialog>
  )
}
