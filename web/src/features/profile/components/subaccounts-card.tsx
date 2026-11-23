import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { Users } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { PasswordInput } from '@/components/password-input'
import { SignOutDialog } from '@/components/sign-out-dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { TitledCard } from '@/components/ui/titled-card'
import { useAccountSecurity } from '@/features/security/hooks/use-account-security'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatQuota } from '@/lib/format'
import { accountPasswordSchema } from '@/lib/password-policy'
import { AuthOperationError } from '@/lib/secure-verification'

import {
  changeSubaccount,
  listSubaccounts,
  type SubaccountAction,
} from '../api'
import type { Subaccount } from '../types'

const createSchema = z.object({
  username: z
    .string()
    .trim()
    .min(1, 'Please enter your username')
    .refine(
      (value) => [...value].length <= 20,
      'Use at most 20 characters for the username.'
    ),
  password: accountPasswordSchema,
})

export function SubaccountsCard(props: { parentId: number }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [creating, setCreating] = useState(false)
  const [target, setTarget] = useState<Subaccount | null>(null)
  const [signOutOpen, setSignOutOpen] = useState(false)
  const security = useAccountSecurity()
  const list = useQuery({
    queryKey: ['self', props.parentId, 'subaccounts'],
    queryFn: ({ signal }) => listSubaccounts(signal),
  })
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { username: '', password: '' },
  })

  async function submit(action: SubaccountAction) {
    const result = await security.run(async (signal) => {
      try {
        return await changeSubaccount(action, signal)
      } catch (error) {
        const failure = AuthOperationError.from(error)
        if (!signal.aborted && failure.code === 'RECENT_LOGIN_REQUIRED') {
          form.resetField('password')
          setTarget(null)
        }
        throw failure
      }
    })
    if (!result) return
    form.reset()
    setCreating(false)
    setTarget(null)
    toast.success(
      action.type === 'create'
        ? t('Subaccount created')
        : t('Subaccount disabled')
    )
    await list.refetch()
  }

  return (
    <TitledCard
      title={t('Subaccounts')}
      icon={<Users aria-hidden='true' />}
      description={t('Usage is charged to the parent wallet.')}
      action={
        !creating && (
          <Button
            size='sm'
            variant='outline'
            disabled={security.pending}
            onClick={() => setCreating(true)}
          >
            {t('Create subaccount')}
          </Button>
        )
      }
    >
      {security.error?.code === 'RECENT_LOGIN_REQUIRED' && (
        <div
          role='alert'
          className='mb-4 flex flex-wrap items-center gap-2 text-sm'
        >
          <span>{t('Please sign in again.')}</span>
          <Button
            size='sm'
            variant='outline'
            onClick={() => setSignOutOpen(true)}
          >
            {t('Sign out')}
          </Button>
        </div>
      )}
      {creating && (
        <form
          className='mb-4 grid gap-3 rounded-lg border p-4'
          onSubmit={form.handleSubmit((data) =>
            submit({ type: 'create', ...data })
          )}
        >
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='grid content-start gap-2'>
              <Label htmlFor='sub-username'>{t('Username')}</Label>
              <Input
                id='sub-username'
                autoComplete='off'
                disabled={security.pending}
                aria-invalid={!!form.formState.errors.username}
                aria-describedby={
                  form.formState.errors.username
                    ? 'sub-username-error'
                    : undefined
                }
                {...form.register('username')}
              />
              {form.formState.errors.username?.message && (
                <p
                  id='sub-username-error'
                  role='alert'
                  className='text-destructive text-sm'
                >
                  {t(form.formState.errors.username.message)}
                </p>
              )}
            </div>
            <div className='grid content-start gap-2'>
              <Label htmlFor='sub-password'>{t('Password')}</Label>
              <PasswordInput
                id='sub-password'
                autoComplete='new-password'
                disabled={security.pending}
                aria-invalid={!!form.formState.errors.password}
                aria-describedby={
                  form.formState.errors.password
                    ? 'sub-password-error'
                    : undefined
                }
                {...form.register('password')}
              />
              {form.formState.errors.password?.message && (
                <p
                  id='sub-password-error'
                  role='alert'
                  className='text-destructive text-sm'
                >
                  {t(form.formState.errors.password.message)}
                </p>
              )}
            </div>
          </div>
          <div className='flex flex-wrap justify-end gap-2'>
            <Button
              type='button'
              variant='ghost'
              disabled={security.pending}
              onClick={() => {
                form.reset()
                setCreating(false)
              }}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={security.pending}>
              {t('Create subaccount')}
            </Button>
          </div>
        </form>
      )}
      {list.isPending && <LoadingState size='sm' />}
      {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
      {list.isSuccess && list.data.length === 0 && (
        <EmptyState
          title={t('No subaccounts yet')}
          className='min-h-32'
          icon={Users}
        />
      )}
      {list.isSuccess && list.data.length > 0 && (
        <ul className='divide-y'>
          {list.data.map((account) => (
            <li
              key={account.id}
              className='flex flex-wrap items-center justify-between gap-3 py-3'
            >
              <div className='min-w-0 space-y-1'>
                <div className='flex flex-wrap items-center gap-2'>
                  <span className='font-medium wrap-anywhere'>
                    {account.username}
                  </span>
                  <StatusBadge
                    label={account.status === 1 ? t('Enabled') : t('Disabled')}
                    variant={account.status === 1 ? 'success' : 'neutral'}
                    copyable={false}
                  />
                </div>
                <dl className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs tabular-nums'>
                  <div className='flex gap-1'>
                    <dt>{t('Total Usage')}</dt>
                    <dd>{formatQuota(account.used_quota)}</dd>
                  </div>
                  <div className='flex gap-1'>
                    <dt>{t('API Requests')}</dt>
                    <dd>{formatNumber(account.request_count, locale)}</dd>
                  </div>
                </dl>
              </div>
              {account.status === 1 && (
                <Button
                  size='sm'
                  variant='outline'
                  aria-label={t('Disable {{username}}', {
                    username: account.username,
                  })}
                  disabled={security.pending}
                  onClick={() => setTarget(account)}
                >
                  {t('Disable')}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={!!target}
        onOpenChange={(open) => {
          if (!open && !security.pending) setTarget(null)
        }}
        title={t('Disable subaccount')}
        desc={t('Disable access for {{username}}?', {
          username: target?.username,
        })}
        confirmText={t('Disable')}
        destructive
        isLoading={security.pending}
        handleConfirm={() => {
          if (target) void submit({ type: 'disable', id: target.id })
        }}
      />
      <SignOutDialog open={signOutOpen} onOpenChange={setSignOutOpen} />
    </TitledCard>
  )
}
