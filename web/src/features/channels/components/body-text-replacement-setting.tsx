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
import { CircleHelp, Plus, Trash2 } from 'lucide-react'
import { useFieldArray, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { ChannelFormValues } from '../lib/channel-form'

export function BodyTextReplacementSetting(props: { disabled?: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const rules = useFieldArray({
    control: form.control,
    name: 'body_text_replacement.rules',
  })

  return (
    <div className='space-y-3 rounded-md border p-3'>
      <FormField
        control={form.control}
        name='body_text_replacement.enabled'
        render={({ field }) => (
          <FormItem className='flex items-center justify-between gap-3'>
            <div className='flex items-center gap-2'>
              <FormLabel>{t('Request body text replacement')}</FormLabel>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-xs'
                      aria-label={t('Request body text replacement')}
                    />
                  }
                >
                  <CircleHelp aria-hidden='true' />
                </TooltipTrigger>
                <TooltipContent className='max-w-80'>
                  {t(
                    'Replace all request body string values, including model names, URLs, nested arrays and multipart text. Case-sensitive, literal rules run in order. Field names, numbers, booleans and file bytes stay unchanged. Leave replacement empty to delete; no rules means no changes.'
                  )}
                </TooltipContent>
              </Tooltip>
            </div>
            <FormControl>
              <Switch
                checked={field.value === true}
                onCheckedChange={field.onChange}
                onBlur={field.onBlur}
                name={field.name}
                ref={field.ref}
                disabled={props.disabled}
              />
            </FormControl>
          </FormItem>
        )}
      />
      {rules.fields.map((rule, index) => (
        <div
          key={rule.id}
          className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-start gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'
        >
          <FormField
            control={form.control}
            name={`body_text_replacement.rules.${index}.from`}
            render={({ field }) => (
              <FormItem className='min-w-0'>
                <FormLabel>{t('Original text')}</FormLabel>
                <FormControl>
                  <Textarea {...field} rows={2} disabled={props.disabled} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name={`body_text_replacement.rules.${index}.to`}
            render={({ field }) => (
              <FormItem className='col-start-1 min-w-0 sm:col-start-auto'>
                <FormLabel>{t('Replace with')}</FormLabel>
                <FormControl>
                  <Textarea {...field} rows={2} disabled={props.disabled} />
                </FormControl>
              </FormItem>
            )}
          />
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            className='col-start-2 row-start-1 mt-6 sm:col-start-3'
            aria-label={t('Delete rule')}
            disabled={props.disabled}
            onClick={() => rules.remove(index)}
          >
            <Trash2 aria-hidden='true' />
          </Button>
        </div>
      ))}
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={props.disabled}
        onClick={() => rules.append({ from: '', to: '' })}
      >
        <Plus aria-hidden='true' />
        {t('Add rule')}
      </Button>
    </div>
  )
}
