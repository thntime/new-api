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
import { Edit, ListFilter, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSystemOptions } from '../hooks/use-system-options'
import { useUpdateOption } from '../hooks/use-update-option'

const OPTION_KEY = 'ErrorStatusMappingRules'

type StatusMappingHeader = { key: string; value: string }

type StatusMappingRule = {
  name: string
  enabled: boolean
  match: { status_code: number; message_regex: string }
  output: {
    status_code: number
    message: string
    type: string
    code: string
    headers: StatusMappingHeader[]
  }
}

function toStatusNumber(value: string): number {
  const parsed = Number.parseInt(value, 10)
  return Number.isNaN(parsed) ? 0 : parsed
}

function normalizeHeader(value: unknown): StatusMappingHeader {
  const header = (value ?? {}) as Partial<StatusMappingHeader>
  return {
    key: typeof header.key === 'string' ? header.key : '',
    value: typeof header.value === 'string' ? header.value : '',
  }
}

function normalizeRule(value: unknown): StatusMappingRule {
  const rule = (value ?? {}) as Partial<StatusMappingRule>
  const match = (rule.match ?? {}) as Partial<StatusMappingRule['match']>
  const output = (rule.output ?? {}) as Partial<StatusMappingRule['output']>
  return {
    name: typeof rule.name === 'string' ? rule.name : '',
    enabled: rule.enabled === true,
    match: {
      status_code: Number(match.status_code) || 0,
      message_regex:
        typeof match.message_regex === 'string' ? match.message_regex : '',
    },
    output: {
      status_code: Number(output.status_code) || 0,
      message: typeof output.message === 'string' ? output.message : '',
      type: typeof output.type === 'string' ? output.type : '',
      code: typeof output.code === 'string' ? output.code : '',
      headers: Array.isArray(output.headers)
        ? output.headers.map(normalizeHeader)
        : [],
    },
  }
}

/** Treats a missing, non-array or unparseable value as an empty rule list. */
function parseRules(raw: string | undefined): {
  rules: StatusMappingRule[]
  invalid: boolean
} {
  if (raw === undefined || raw.trim() === '') {
    return { rules: [], invalid: false }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return { rules: [], invalid: true }
  }
  if (!Array.isArray(parsed)) return { rules: [], invalid: true }
  return { rules: parsed.map(normalizeRule), invalid: false }
}

type RuleFormValues = {
  name: string
  enabled: boolean
  match_status_code: string
  match_message_regex: string
  output_status_code: string
  output_message: string
  output_type: string
  output_code: string
}

type RuleDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  rule: StatusMappingRule | null
  onSave: (rule: StatusMappingRule) => void
}

type HeaderRow = StatusMappingHeader & { rowId: string }

const RULE_FORM_ID = 'status-mapping-rule-form'

const emptyRuleFormValues: RuleFormValues = {
  name: '',
  enabled: false,
  match_status_code: '0',
  match_message_regex: '',
  output_status_code: '',
  output_message: '',
  output_type: '',
  output_code: '',
}

function StatusMappingRuleDialog(props: RuleDialogProps) {
  const { t } = useTranslation()
  const nextRowId = useRef(0)
  const createHeaderRow = (header?: StatusMappingHeader): HeaderRow => ({
    key: header?.key ?? '',
    value: header?.value ?? '',
    rowId: String(nextRowId.current++),
  })
  const [headers, setHeaders] = useState<HeaderRow[]>([])
  const form = useForm<RuleFormValues>({ defaultValues: emptyRuleFormValues })

  useEffect(() => {
    if (!props.open) return
    const rule = props.rule
    form.reset({
      name: rule?.name ?? '',
      enabled: rule?.enabled ?? false,
      match_status_code: rule ? String(rule.match.status_code) : '0',
      match_message_regex: rule?.match.message_regex ?? '',
      output_status_code: rule ? String(rule.output.status_code) : '',
      output_message: rule?.output.message ?? '',
      output_type: rule?.output.type ?? '',
      output_code: rule?.output.code ?? '',
    })
    setHeaders(rule ? rule.output.headers.map(createHeaderRow) : [])
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open, props.rule])

  const handleSave = (values: RuleFormValues) => {
    props.onSave({
      name: values.name.trim(),
      enabled: values.enabled,
      match: {
        status_code: toStatusNumber(values.match_status_code),
        message_regex: values.match_message_regex.trim(),
      },
      output: {
        status_code: toStatusNumber(values.output_status_code),
        message: values.output_message.trim(),
        type: values.output_type.trim(),
        code: values.output_code.trim(),
        headers: headers
          .filter((header) => header.key.trim() !== '')
          .map((header) => ({ key: header.key.trim(), value: header.value })),
      },
    })
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.rule ? t('Edit Rule') : t('New Rule')}
      contentClassName='sm:max-w-3xl'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={RULE_FORM_ID}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <form
        id={RULE_FORM_ID}
        onSubmit={form.handleSubmit(handleSave)}
        className='min-w-0 space-y-4'
      >
        <div className='grid gap-1.5'>
          <Label htmlFor='status-mapping-rule-name'>{t('Name')}</Label>
          <Input id='status-mapping-rule-name' {...form.register('name')} />
        </div>

        <div className='flex items-center justify-between gap-4'>
          <Label htmlFor='status-mapping-rule-enabled'>{t('Enabled')}</Label>
          <Switch
            id='status-mapping-rule-enabled'
            checked={form.watch('enabled')}
            onCheckedChange={(checked) =>
              form.setValue('enabled', checked, { shouldDirty: true })
            }
          />
        </div>

        <Separator />

        <h4 className='text-sm font-medium'>{t('Match')}</h4>
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='grid gap-1.5'>
            <Label htmlFor='status-mapping-rule-match-status'>
              {t('Match status code')}
            </Label>
            <Input
              id='status-mapping-rule-match-status'
              type='number'
              min={0}
              max={599}
              placeholder='0'
              {...form.register('match_status_code')}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='status-mapping-rule-match-regex'>
              {t('Message regex')}
            </Label>
            <Input
              id='status-mapping-rule-match-regex'
              placeholder='exhausted|tpm/rpm'
              {...form.register('match_message_regex')}
            />
          </div>
        </div>

        <Separator />

        <h4 className='text-sm font-medium'>{t('Output')}</h4>
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='grid gap-1.5'>
            <Label htmlFor='status-mapping-rule-output-status'>
              {t('Output status code')}
            </Label>
            <Input
              id='status-mapping-rule-output-status'
              type='number'
              min={100}
              max={599}
              placeholder='429'
              {...form.register('output_status_code')}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='status-mapping-rule-output-message'>
              {t('Message')}
            </Label>
            <Input
              id='status-mapping-rule-output-message'
              {...form.register('output_message')}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='status-mapping-rule-output-type'>{t('Type')}</Label>
            <Input
              id='status-mapping-rule-output-type'
              placeholder='rate_limit_error'
              {...form.register('output_type')}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='status-mapping-rule-output-code'>
              {t('Error code')}
            </Label>
            <Input
              id='status-mapping-rule-output-code'
              placeholder='rate_limit_exceeded'
              {...form.register('output_code')}
            />
          </div>
        </div>

        <Separator />

        <div className='space-y-2'>
          <div className='flex items-center justify-between'>
            <Label>{t('Response headers')}</Label>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setHeaders((prev) => [...prev, createHeaderRow()])}
            >
              <Plus aria-hidden='true' />
              {t('Add header')}
            </Button>
          </div>
          {headers.map((header) => (
            <div key={header.rowId} className='flex min-w-0 items-center gap-2'>
              <Input
                aria-label={t('Key')}
                placeholder='x-should-retry'
                value={header.key}
                onChange={(event) =>
                  setHeaders((prev) =>
                    prev.map((item) =>
                      item.rowId === header.rowId
                        ? { ...item, key: event.target.value }
                        : item
                    )
                  )
                }
              />
              <Input
                aria-label={t('Value')}
                placeholder='true'
                value={header.value}
                onChange={(event) =>
                  setHeaders((prev) =>
                    prev.map((item) =>
                      item.rowId === header.rowId
                        ? { ...item, value: event.target.value }
                        : item
                    )
                  )
                }
              />
              <Button
                type='button'
                variant='ghost'
                size='icon'
                aria-label={t('Delete')}
                onClick={() =>
                  setHeaders((prev) =>
                    prev.filter((item) => item.rowId !== header.rowId)
                  )
                }
              >
                <Trash2 aria-hidden='true' />
              </Button>
            </div>
          ))}
        </div>
      </form>
    </Dialog>
  )
}

export function StatusMappingSection() {
  const { t } = useTranslation()
  const query = useSystemOptions()
  const updateOption = useUpdateOption()
  const raw = query.data?.data.find(
    (option) => option.key === OPTION_KEY
  )?.value
  const parsed = useMemo(() => parseRules(raw), [raw])
  const [rules, setRules] = useState<StatusMappingRule[]>(parsed.rules)
  const [baseline, setBaseline] = useState(() => JSON.stringify(parsed.rules))
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [deletingIndex, setDeletingIndex] = useState<number | null>(null)

  useEffect(() => {
    setRules(parsed.rules)
    setBaseline(JSON.stringify(parsed.rules))
  }, [parsed])

  const save = async () => {
    const current = JSON.stringify(rules)
    if (current === baseline) {
      toast.info(t('No changes to save'))
      return
    }
    try {
      await updateOption.mutateAsync({ key: OPTION_KEY, value: current })
      setBaseline(current)
    } catch {
      // The mutation reports the server error below and in a toast.
    }
  }

  const handleRuleSave = (rule: StatusMappingRule) => {
    setRules((prev) => {
      if (editingIndex === null) return [...prev, rule]
      return prev.map((item, index) => (index === editingIndex ? rule : item))
    })
    setEditingIndex(null)
  }

  return (
    <SettingsSection title={t('Status mapping')}>
      <SettingsPageFormActions
        onSave={() => void save()}
        isSaving={updateOption.isPending}
      />
      <div className='text-muted-foreground space-y-1 text-sm'>
        <p>
          {t(
            'Rewrite matching upstream errors into a response that clients can retry.'
          )}
        </p>
        <p>{t('Rules are evaluated in order; the first match wins.')}</p>
      </div>
      {parsed.invalid ? (
        <Alert>
          <AlertDescription>
            {t('Existing rules could not be parsed and are treated as empty.')}
          </AlertDescription>
        </Alert>
      ) : null}
      {updateOption.isError ? (
        <p role='alert' className='text-destructive text-sm'>
          {updateOption.error.message}
        </p>
      ) : null}
      <div className='flex justify-end'>
        <Button
          onClick={() => {
            setEditingIndex(null)
            setEditorOpen(true)
          }}
        >
          <Plus aria-hidden='true' />
          {t('Add Rule')}
        </Button>
      </div>
      {rules.length === 0 ? (
        <EmptyState
          icon={ListFilter}
          title={t('No rules configured')}
          description={t('Add a rule to map matching upstream errors.')}
        />
      ) : (
        <StaticDataTable
          containerProps={{
            role: 'region',
            'aria-label': t('Status mapping rules'),
          }}
          data={rules}
          getRowKey={(_, index) => index}
          columns={[
            {
              id: 'enabled',
              header: t('Enabled'),
              className: 'w-24',
              cell: (rule) =>
                rule.enabled ? (
                  <Badge variant='secondary'>{t('Enabled')}</Badge>
                ) : (
                  <span className='text-muted-foreground'>—</span>
                ),
            },
            {
              id: 'name',
              header: t('Name'),
              cell: (rule) => rule.name || '—',
            },
            {
              id: 'match',
              header: t('Match'),
              cell: (rule) => (
                <div className='flex min-w-0 flex-col gap-1'>
                  <span>
                    {rule.match.status_code === 0
                      ? t('Any status code')
                      : rule.match.status_code}
                  </span>
                  <code className='text-muted-foreground'>
                    {rule.match.message_regex || '—'}
                  </code>
                </div>
              ),
            },
            {
              id: 'output',
              header: t('Output'),
              cell: (rule) => (
                <div className='flex min-w-0 flex-col gap-1'>
                  <span>{rule.output.status_code}</span>
                  <span className='text-muted-foreground'>
                    {rule.output.message || '—'}
                  </span>
                </div>
              ),
            },
            {
              id: 'headers',
              header: t('Headers'),
              className: 'w-24 text-right',
              cellClassName: 'text-right tabular-nums',
              cell: (rule) => rule.output.headers.length,
            },
            {
              id: 'actions',
              header: t('Actions'),
              className: 'w-24 text-right',
              cell: (_, index) => (
                <div className='flex items-center justify-end gap-1'>
                  <Button
                    variant='ghost'
                    size='icon'
                    aria-label={t('Edit Rule')}
                    onClick={() => {
                      setEditingIndex(index)
                      setEditorOpen(true)
                    }}
                  >
                    <Edit aria-hidden='true' />
                  </Button>
                  <Button
                    variant='ghost'
                    size='icon'
                    aria-label={t('Delete Rule')}
                    onClick={() => setDeletingIndex(index)}
                  >
                    <Trash2 aria-hidden='true' />
                  </Button>
                </div>
              ),
            },
          ]}
        />
      )}

      <StatusMappingRuleDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        rule={editingIndex === null ? null : rules[editingIndex]}
        onSave={handleRuleSave}
      />

      {deletingIndex !== null && (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setDeletingIndex(null)}
          title={t('Delete Rule')}
          desc={`${t('Delete rule “{{name}}”?', {
            name: rules[deletingIndex]?.name || '—',
          })} ${t('Save your changes to apply the removal.')}`}
          confirmText={t('Delete')}
          destructive
          handleConfirm={() => {
            setRules((prev) =>
              prev.filter((_, index) => index !== deletingIndex)
            )
            setDeletingIndex(null)
          }}
        />
      )}
    </SettingsSection>
  )
}
