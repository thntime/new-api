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
import { Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import { exportChannels } from '../../api'
import {
  downloadChannelExport,
  validateChannelExportPassword,
} from '../../lib/channel-export'

interface ExportChannelsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  ids: number[]
  onSuccess?: () => void
}

export function ExportChannelsDialog(props: ExportChannelsDialogProps) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleOpenChange = (open: boolean) => {
    if (!open) setPassword('')
    props.onOpenChange(open)
  }

  const handleExport = async () => {
    if (props.ids.length === 0) {
      toast.error(t('Please select at least one channel'))
      return
    }
    const passwordError = validateChannelExportPassword(password)
    if (passwordError) {
      toast.error(t(passwordError))
      return
    }

    setIsSubmitting(true)
    try {
      const response = await exportChannels({ ids: props.ids, password })
      if (!response.success || !response.data) {
        handleServerError(response, t('Failed to export channels'))
        return
      }
      downloadChannelExport(response.data.filename, response.data.content)
      toast.success(
        t('Exported {{count}} channels', { count: props.ids.length })
      )
      setPassword('')
      props.onSuccess?.()
      props.onOpenChange(false)
    } catch (error) {
      handleServerError(error, t('Failed to export channels'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleOpenChange}
      title={t('Export Channels')}
      description={t(
        'The exported file is encrypted with this password. The password cannot be recovered.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => handleOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleExport} disabled={isSubmitting}>
            {isSubmitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Export')}
          </Button>
        </>
      }
    >
      <div className='grid gap-4 py-4'>
        <div className='grid gap-2'>
          <Label htmlFor='channel-export-password'>{t('Password')}</Label>
          <Input
            id='channel-export-password'
            type='password'
            autoComplete='new-password'
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Password must contain between 8 and 128 characters.')}
          </p>
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('The file contains channel keys. Store it securely.')}
        </p>
      </div>
    </Dialog>
  )
}
