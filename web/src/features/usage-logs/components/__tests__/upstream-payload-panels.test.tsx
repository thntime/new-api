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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import { api } from '@/lib/api'

import { UpstreamPayloadPanels } from '../dialogs/upstream-payload-panels'

const i18n = createInstance()
let client: QueryClient

beforeEach(async () => {
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

function renderPanels(requestId = 'req-1') {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <UpstreamPayloadPanels requestId={requestId} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('collapsed panels do not fetch until expanded', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: null },
  }))
  renderPanels()
  expect(screen.getByText('Upstream Request')).toBeVisible()
  expect(screen.getByText('Upstream Response')).toBeVisible()
  expect(get).not.toHaveBeenCalled()

  await userEvent.click(
    screen.getByRole('button', {
      name: 'Show upstream payload — Upstream Request',
    })
  )
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/log/upstream_payload?request_id=req-1',
      expect.objectContaining({ skipErrorHandler: true })
    )
  )
})

test('an expanded response panel shows the SSE source verbatim', async () => {
  const sse = 'event: message\ndata: {"id":"1"}\n\ndata: [DONE]\n'
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: {
      success: true,
      data: {
        request_body: '{"model":"gpt-4o"}',
        response_body: sse,
        request_truncated: false,
        response_truncated: false,
      },
    },
  }))
  renderPanels()
  await userEvent.click(
    screen.getByRole('button', {
      name: 'Show upstream payload — Upstream Response',
    })
  )
  await waitFor(() =>
    expect(document.body.textContent).toContain('data: [DONE]')
  )
  // The SSE body must reach the viewer verbatim: no trimming, no reformatting.
  expect(document.body.textContent).toContain('data: {"id":"1"}')
})

test('a compact JSON body is pretty printed for readability', async () => {
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: {
      success: true,
      data: {
        request_body: '{"a":1,"b":[1,2]}',
        response_body: '',
        request_truncated: false,
        response_truncated: false,
      },
    },
  }))
  renderPanels()
  await userEvent.click(
    screen.getByRole('button', {
      name: 'Show upstream payload — Upstream Request',
    })
  )
  await waitFor(() =>
    expect(document.body.textContent).toContain('"a": 1')
  )
})

test('an in-flight request shows the loading placeholder', async () => {
  let respond: (value: unknown) => void = () => undefined
  vi.spyOn(api, 'get').mockImplementation(
    () =>
      new Promise((resolve) => {
        respond = resolve
      })
  )
  renderPanels()
  await userEvent.click(
    screen.getByRole('button', {
      name: 'Show upstream payload — Upstream Request',
    })
  )
  expect(await screen.findByText('Loading upstream payload...')).toBeVisible()
  respond({ data: { success: true, data: null } })
})

test('a failed request shows the failure placeholder', async () => {
  vi.spyOn(api, 'get').mockImplementation(async () => {
    throw new Error('upstream payload request failed')
  })
  renderPanels()
  await userEvent.click(
    screen.getByRole('button', {
      name: 'Show upstream payload — Upstream Request',
    })
  )
  expect(
    await screen.findByText('Failed to load the upstream payload.')
  ).toBeVisible()
})

test('a truncated body shows the truncation notice', async () => {
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: {
      success: true,
      data: {
        request_body: '{"model":"gpt-4o"}',
        response_body: 'data: partial',
        request_truncated: true,
        response_truncated: true,
      },
    },
  }))
  renderPanels()
  await userEvent.click(
    screen.getByRole('button', {
      name: 'Show upstream payload — Upstream Request',
    })
  )
  expect(
    await screen.findByText(
      'This payload was truncated at the capture size limit.'
    )
  ).toBeVisible()
})

test('no recorded payload shows the empty placeholder without an error', async () => {
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: null },
  }))
  renderPanels()
  await userEvent.click(
    screen.getByRole('button', {
      name: 'Show upstream payload — Upstream Request',
    })
  )
  expect(
    await screen.findByText(
      'No upstream payload was recorded for this request.'
    )
  ).toBeVisible()
})

test('a recorded payload with an empty request body shows a dash, not the no-record placeholder', async () => {
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: {
      success: true,
      data: {
        request_body: '',
        response_body: '{"ok":true}',
        request_truncated: false,
        response_truncated: false,
      },
    },
  }))
  renderPanels()
  await userEvent.click(
    screen.getByRole('button', {
      name: 'Show upstream payload — Upstream Request',
    })
  )
  expect(await screen.findByText('-')).toBeVisible()
  expect(
    screen.queryByText('No upstream payload was recorded for this request.')
  ).toBeNull()
})
