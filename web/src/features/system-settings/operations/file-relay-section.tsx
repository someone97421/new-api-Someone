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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
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
import { Switch } from '@/components/ui/switch'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  SettingsForm,
  SettingsFormGrid,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  createFileRelaySchema,
  fileRelayDefaults,
  type FileRelayValues,
} from './lib/file-relay'

export function FileRelaySection(props: { value: string }) {
  const { t } = useTranslation()
  let values: FileRelayValues
  try {
    values = createFileRelaySchema(t).parse(
      props.value ? JSON.parse(props.value) : fileRelayDefaults
    )
    values.auto_relay_enabled ??= values.enabled
  } catch {
    return (
      <ErrorState
        title={t('Invalid file relay configuration')}
        description={t(
          'FileRelaySettings could not be read. Correct the stored configuration and reload this page.'
        )}
      />
    )
  }
  return <FileRelayForm key={props.value} defaultValues={values} />
}

function FileRelayForm(props: { defaultValues: FileRelayValues }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<FileRelayValues>({
    resolver: zodResolver(createFileRelaySchema(t)),
    defaultValues: props.defaultValues,
  })
  const isSaving = form.formState.isSubmitting

  const onSubmit = async (values: FileRelayValues) => {
    form.clearErrors('root')
    try {
      await updateOption.mutateAsync({
        key: 'FileRelaySettings',
        value: JSON.stringify(values),
      })
      form.reset(values)
    } catch (error) {
      form.setError('root', {
        message: getServerErrorMessage(error, t('Failed to update setting')),
      })
    }
  }

  return (
    <SettingsSection title={t('File relay')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={isSaving}
            isSaveDisabled={!form.formState.isDirty}
            onReset={() => form.reset()}
            isResetDisabled={!form.formState.isDirty}
          />
          <p className='text-muted-foreground text-sm'>
            {t(
              'Store remote URLs, Base64 data and uploaded files locally, then return a URL on this site. Automatic relay covers non-streaming image results and task artifacts cached on first access.'
            )}
          </p>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Submit URLs, Base64 data or files to POST /v1/file-relay using your existing API key.'
            )}
          </p>
          {form.formState.errors.root && (
            <p role='alert' className='text-destructive text-sm'>
              {form.formState.errors.root.message}
            </p>
          )}
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable direct file uploads')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Allow API clients to upload files through POST /v1/file-relay.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={isSaving}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <FormField
            control={form.control}
            name='auto_relay_enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Automatic file relay by default')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Channels inherit this default unless overridden. A channel can enable automatic relay even when this default is off.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value ?? false}
                    onCheckedChange={field.onChange}
                    disabled={isSaving}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <fieldset disabled={isSaving} className='min-w-0'>
            <SettingsFormGrid>
              <FormField
                control={form.control}
                name='directory'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Storage directory')}</FormLabel>
                    <FormControl>
                      <Input {...field} />
                    </FormControl>
                    <FormDescription>
                      {t('Path on the server used to store relayed files.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='public_url'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Public site base URL')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='url'
                        placeholder='https://example.com'
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Leave blank to use TaskPublicAddress, then ServerAddress. The server appends /v1/file-relay/content/:id.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='retention_hours'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('File retention (hours)')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={0}
                        max={87600}
                        step={1}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? Number.NaN
                              : Number(event.target.value)
                          )
                        }
                        value={Number.isNaN(field.value) ? '' : field.value}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Expired files are deleted during cleanup. Set to 0 to keep files permanently.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='cleanup_interval_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Cleanup interval (minutes)')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={1}
                        max={1440}
                        step={1}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? Number.NaN
                              : Number(event.target.value)
                          )
                        }
                        value={Number.isNaN(field.value) ? '' : field.value}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='download_timeout_seconds'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Download timeout (seconds)')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={1}
                        max={3600}
                        step={1}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? Number.NaN
                              : Number(event.target.value)
                          )
                        }
                        value={Number.isNaN(field.value) ? '' : field.value}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='retry_count'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Download retries')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={0}
                        max={5}
                        step={1}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? Number.NaN
                              : Number(event.target.value)
                          )
                        }
                        value={Number.isNaN(field.value) ? '' : field.value}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Number of retries after a failed download (0–5).')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='strict'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>
                        {t('Require local hosting by default')}
                      </FormLabel>
                      <FormDescription>
                        {t(
                          'By default, relay failures keep the original result. Strict mode returns a delivery error while completed generation remains billed. Channels can override this policy.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={isSaving}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </SettingsFormGrid>
          </fieldset>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
