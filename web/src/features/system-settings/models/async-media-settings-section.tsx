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
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  asyncMediaSettingsSchema,
  normalizeAsyncMediaSettings,
  type AsyncMediaSettingsFormValues,
} from './async-media-settings'

type AsyncMediaSettingsSectionProps = {
  defaultValues: AsyncMediaSettingsFormValues
}

export function AsyncMediaSettingsSection(
  props: AsyncMediaSettingsSectionProps
) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const settingsForm = useSettingsForm<AsyncMediaSettingsFormValues>({
    resolver: zodResolver(asyncMediaSettingsSchema),
    defaultValues: props.defaultValues,
    onSubmit: async (values, changedFields) => {
      const normalized = normalizeAsyncMediaSettings(values)
      for (const key of Object.keys(changedFields) as Array<
        keyof AsyncMediaSettingsFormValues
      >) {
        await updateOption.mutateAsync({ key, value: normalized[key] })
      }
    },
  })

  return (
    <SettingsSection title={t('Async Media')}>
      <Form {...settingsForm.form}>
        <SettingsForm onSubmit={settingsForm.handleSubmit}>
          <SettingsPageFormActions
            onSave={settingsForm.handleSubmit}
            isSaving={updateOption.isPending}
          />

          <Alert>
            <AlertTitle>{t('Restart required')}</AlertTitle>
            <AlertDescription>
              {t(
                'Storage directory and worker count take effect after all instances restart. Other settings apply to new work immediately.'
              )}
            </AlertDescription>
          </Alert>

          <Alert>
            <AlertTitle>{t('Beginner protocol bridge setup')}</AlertTitle>
            <AlertDescription>
              {t(
                'Create Gemini2GPT and GPT2Gemini channels, keep channels for the same public model in one group, and use priority to choose the primary route. This page controls the shared async queue and failover attempts.'
              )}
            </AlertDescription>
          </Alert>

          <FormField
            control={settingsForm.form.control}
            name='AsyncMediaEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Accept new async media jobs')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Allow requests with async=true to be queued. Existing jobs continue processing when disabled.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <Separator />

          <div className='flex min-w-0 flex-col gap-1'>
            <h4 className='text-sm font-medium'>{t('Routing failover')}</h4>
            <p className='text-muted-foreground text-xs'>
              {t(
                'A retry selects another available channel before an upstream task ID is accepted.'
              )}
            </p>
          </div>

          <FormField
            control={settingsForm.form.control}
            name='RetryTimes'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Failover retry count')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='0'
                    max='10'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Use 1 for a primary and backup channel. Use 0 to disable failover.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Separator />

          <div className='flex min-w-0 flex-col gap-1'>
            <h4 className='text-sm font-medium'>
              {t('Storage and execution')}
            </h4>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Manage persistent files, background concurrency, and worker ownership.'
              )}
            </p>
          </div>

          <FormField
            control={settingsForm.form.control}
            name='AsyncMediaStoragePath'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel>{t('Storage directory')}</FormLabel>
                <FormControl>
                  <Input
                    className='font-mono'
                    placeholder='./data/async-media'
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Directory used for async request, response, and result files.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={settingsForm.form.control}
            name='AsyncMediaWorkers'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Worker count')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='1'
                    max='128'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Number of background workers started per instance.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={settingsForm.form.control}
            name='AsyncMediaLeaseSeconds'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Worker lease (seconds)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='30'
                    max='3600'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'How long a worker owns a job before its lease is considered expired.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Separator />

          <div className='flex min-w-0 flex-col gap-1'>
            <h4 className='text-sm font-medium'>{t('Lifecycle limits')}</h4>
            <p className='text-muted-foreground text-xs'>
              {t('Control result retention, file size, and task lifetime.')}
            </p>
          </div>

          <FormField
            control={settingsForm.form.control}
            name='AsyncMediaRetentionHours'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Result retention (hours)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='1'
                    max='8760'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t('How long completed media files remain downloadable.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={settingsForm.form.control}
            name='AsyncMediaMaxFileMB'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Maximum file size (MB)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='1'
                    max='10240'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Reject a single downloaded or decoded result above this size.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={settingsForm.form.control}
            name='TaskTimeoutMinutes'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Task timeout (minutes)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='0'
                    max='525600'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Maximum total task age. Use 0 to disable the timeout.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
