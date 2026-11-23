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
import { useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import { importChannels } from '../../api'
import { channelsQueryKeys } from '../../lib'
import {
  readChannelExportFile,
  validateChannelExportPassword,
} from '../../lib/channel-export'
import type { ImportChannelsResponse } from '../../types'

interface ImportChannelsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function ImportChannelsDialog(props: ImportChannelsDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [password, setPassword] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const clearForm = () => {
    setPassword('')
    setFile(null)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  const handleOpenChange = (open: boolean) => {
    if (!open) clearForm()
    props.onOpenChange(open)
  }

  // The backend nests the translated inner failure in message_params so a
  // rejected channel keeps its own localized reason.
  const describeImportFailure = (response: ImportChannelsResponse): string => {
    const params = response.message_params ?? {}
    let errorText: string | undefined
    if (typeof params.error_key === 'string') {
      const errorParams =
        typeof params.error_params === 'object' && params.error_params !== null
          ? (params.error_params as Record<string, unknown>)
          : undefined
      errorText = t(params.error_key, errorParams)
    } else if (typeof params.error === 'string') {
      errorText = params.error
    } else {
      errorText = response.message
    }

    if (typeof response.message_key === 'string' && response.message_key) {
      return t(response.message_key, { ...params, error: errorText })
    }
    return response.message || t('Failed to import channels')
  }

  const handleImport = async () => {
    const passwordError = validateChannelExportPassword(password)
    if (passwordError) {
      toast.error(t(passwordError))
      return
    }
    if (!file) {
      toast.error(t('Please select a channel export file'))
      return
    }

    setIsSubmitting(true)
    try {
      const content = await readChannelExportFile(file)
      const response = await importChannels({ password, content })
      if (!response.success || !response.data) {
        toast.error(describeImportFailure(response))
        return
      }
      toast.success(
        t('Imported {{count}} channels', { count: response.data.count })
      )
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      clearForm()
      props.onSuccess?.()
      props.onOpenChange(false)
    } catch (error) {
      handleServerError(error, t('Failed to import channels'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleOpenChange}
      title={t('Import Channels')}
      description={t(
        'Import channels from a password-encrypted channel export file.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => handleOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleImport} disabled={isSubmitting}>
            {isSubmitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Import')}
          </Button>
        </>
      }
    >
      <div className='grid gap-4 py-4'>
        <div className='grid gap-2'>
          <Label htmlFor='channel-import-file'>
            {t('Channel export file')}
          </Label>
          <Input
            ref={fileInputRef}
            id='channel-import-file'
            type='file'
            accept='.nacx,application/json'
            onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          />
          {file ? (
            <p className='text-muted-foreground text-xs'>{file.name}</p>
          ) : null}
        </div>
        <div className='grid gap-2'>
          <Label htmlFor='channel-import-password'>{t('Password')}</Label>
          <Input
            id='channel-import-password'
            type='password'
            autoComplete='new-password'
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>
        <p className='text-muted-foreground text-xs'>
          {t(
            'The file is decrypted with the password used when it was exported.'
          )}
        </p>
      </div>
    </Dialog>
  )
}
