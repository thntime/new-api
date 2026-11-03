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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { type UseFormReturn, useForm } from 'react-hook-form'
import { afterEach, expect, it, vi } from 'vitest'

import { SettingsPageProvider } from '@/features/system-settings/components/settings-page-context'
import { ModelRatioForm } from '@/features/system-settings/models/model-ratio-form'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

type FormValues = {
  ModelPrice: string
  ModelRatio: string
  CacheRatio: string
  CreateCacheRatio: string
  CompletionRatio: string
  ImageRatio: string
  AudioRatio: string
  AudioCompletionRatio: string
  ExposeRatioEnabled: boolean
  BillingMode: string
  BillingExpr: string
  PluginBillingExpr: string
}

const clients: QueryClient[] = []
let currentForm: UseFormReturn<FormValues> | undefined

const baseValues: FormValues = {
  ModelPrice: '{}',
  ModelRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  ExposeRatioEnabled: false,
  BillingMode: '{}',
  BillingExpr: '{}',
  PluginBillingExpr: '{}',
}

afterEach(() => {
  cleanup()
  for (const client of clients) client.clear()
  clients.length = 0
  currentForm = undefined
  localStorage.removeItem('model-ratio-column-visibility')
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

function FormFixture(props: {
  values: FormValues
  enabledModels?: string[]
  variant?: 'default' | 'unset'
  onSave: () => Promise<void>
}) {
  const [actionsContainer, setActionsContainer] =
    useState<HTMLDivElement | null>(null)
  const form = useForm<FormValues>({ defaultValues: props.values })
  currentForm = form
  return (
    <>
      <header>
        <div ref={setActionsContainer} />
      </header>
      <SettingsPageProvider actionsContainer={actionsContainer}>
        <ModelRatioForm
          form={form}
          savedValues={props.values}
          variant={props.variant ?? 'default'}
          onSave={props.onSave}
          onReset={() => undefined}
          isSaving={false}
          isResetting={false}
        />
      </SettingsPageProvider>
    </>
  )
}

function renderForm(
  values: Partial<FormValues>,
  options: {
    enabledModels?: string[]
    variant?: 'default' | 'unset'
  } = {}
) {
  const onSave = vi.fn(async () => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/models_enabled') {
      return {
        data: { success: true, data: options.enabledModels ?? [] },
      }
    }
    if (url === '/api/option/model_pricing') {
      return {
        data: {
          success: true,
          data: { entries: [], options: {}, empty_version: '' },
        },
      }
    }
    return { data: { success: true, data: [], vendors: [] } }
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <FormFixture
        values={{ ...baseValues, ...values }}
        enabledModels={options.enabledModels}
        variant={options.variant}
        onSave={onSave}
      />
    </QueryClientProvider>
  )
  return { onSave, client }
}

const priceSummaryValues: Partial<FormValues> = {
  ModelPrice: JSON.stringify({ 'priced-model': 0.1 }),
  ModelRatio: JSON.stringify({ 'ratio-model': 2 }),
  CompletionRatio: JSON.stringify({ orphan: 1.2 }),
  BillingMode: JSON.stringify({
    'expr-model': 'tiered_expr',
    orphan: 'ratio',
  }),
  BillingExpr: JSON.stringify({ 'expr-model': 'tier("base", fixed(1))' }),
}

it('shows only models with base pricing in the main list', async () => {
  const { client } = renderForm(priceSummaryValues)
  await waitFor(() => expect(client.isFetching()).toBe(0))

  for (const name of ['priced-model', 'ratio-model', 'expr-model']) {
    expect(
      await screen.findByRole('row', { name: new RegExp(name) })
    ).toBeInTheDocument()
  }
  expect(screen.queryByRole('row', { name: /orphan/ })).not.toBeInTheDocument()
})

it('keeps the unset tab semantics: enabled models without base pricing', async () => {
  const { client } = renderForm(priceSummaryValues, {
    variant: 'unset',
    enabledModels: ['priced-model', 'orphan'],
  })
  await waitFor(() => expect(client.isFetching()).toBe(0))

  expect(await screen.findByRole('row', { name: /orphan/ })).toBeInTheDocument()
  expect(
    screen.queryByRole('row', { name: /priced-model/ })
  ).not.toBeInTheDocument()
})

const clearValues: Partial<FormValues> = {
  ModelPrice: JSON.stringify({ 'model-a': 0.1, 'model-b': 0.2 }),
  ModelRatio: JSON.stringify({ 'model-a': 2, 'model-b': 3 }),
  CacheRatio: JSON.stringify({ 'model-a': 1.1 }),
  CreateCacheRatio: JSON.stringify({ 'model-a': 1.2 }),
  CompletionRatio: JSON.stringify({ 'model-a': 1.3 }),
  ImageRatio: JSON.stringify({ 'model-a': 1.4 }),
  AudioRatio: JSON.stringify({ 'model-a': 1.5 }),
  AudioCompletionRatio: JSON.stringify({ 'model-a': 1.6 }),
  BillingMode: JSON.stringify({ 'model-a': 'tiered_expr' }),
  BillingExpr: JSON.stringify({ 'model-a': 'tier("base", fixed(1))' }),
  PluginBillingExpr: JSON.stringify({ 'plug::model-a': 'u("seconds") * 1' }),
}

async function selectModelRows(names: string[]) {
  for (const name of names) {
    const row = await screen.findByRole('row', { name: new RegExp(name) })
    await userEvent.click(within(row).getByRole('checkbox'))
  }
}

it('places the clear button next to Reset prices and disables it without a selection', async () => {
  const { client } = renderForm(clearValues)
  await waitFor(() => expect(client.isFetching()).toBe(0))

  const reset = screen.getByRole('button', { name: 'Reset prices' })
  const clear = screen.getByRole('button', { name: 'Clear' })
  expect(clear).toBeDisabled()
  expect(reset.parentElement).toContainElement(clear)
  expect(
    reset.compareDocumentPosition(clear) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()

  await selectModelRows(['model-a'])
  await waitFor(() => expect(clear).toBeEnabled())
})

it('confirms the selected count and clears every pricing field as a draft', async () => {
  const { client, onSave } = renderForm(clearValues)
  await waitFor(() => expect(client.isFetching()).toBe(0))
  const apiWrite = vi.spyOn(api, 'put')
  const apiPatch = vi.spyOn(api, 'patch')

  await selectModelRows(['model-a', 'model-b'])
  const clearButton = screen.getByRole('button', { name: 'Clear' })
  await waitFor(() => expect(clearButton).toBeEnabled())
  await userEvent.click(clearButton)

  const dialog = await screen.findByRole('alertdialog')
  expect(dialog).toHaveTextContent('Clear pricing for 2 models?')
  await userEvent.click(within(dialog).getByRole('button', { name: 'Clear' }))

  await waitFor(() =>
    expect(
      screen.queryByRole('row', { name: /model-a/ })
    ).not.toBeInTheDocument()
  )
  expect(screen.queryByRole('row', { name: /model-b/ })).not.toBeInTheDocument()

  const values = currentForm?.getValues()
  for (const field of [
    'ModelPrice',
    'ModelRatio',
    'CacheRatio',
    'CreateCacheRatio',
    'CompletionRatio',
    'ImageRatio',
    'AudioRatio',
    'AudioCompletionRatio',
    'BillingMode',
    'BillingExpr',
  ] as const) {
    expect(JSON.parse(values?.[field] ?? '{}')).toEqual({})
  }
  expect(JSON.parse(values?.PluginBillingExpr ?? '{}')).toEqual({})
  expect(onSave).not.toHaveBeenCalled()
  expect(apiWrite).not.toHaveBeenCalled()
  expect(apiPatch).not.toHaveBeenCalled()
})

it('does not offer the clear button in JSON mode', async () => {
  const { client } = renderForm(clearValues)
  await waitFor(() => expect(client.isFetching()).toBe(0))

  await userEvent.click(screen.getByRole('button', { name: 'Switch to JSON' }))

  expect(
    screen.queryByRole('button', { name: 'Clear' })
  ).not.toBeInTheDocument()
})
