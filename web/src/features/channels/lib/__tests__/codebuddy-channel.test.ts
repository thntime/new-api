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
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_PROVIDER_PRESENTATION,
  CHANNEL_TYPE_CODEBUDDY,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPES,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import { CHANNEL_TYPE_DEFAULTS } from '../channel-type-defaults'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'
import { getChannelTypeIcon } from '../channel-utils'
import { channelSchema } from '../../types'

function codebuddyForm(overrides: Record<string, unknown> = {}) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'CodeBuddy upstream',
    type: CHANNEL_TYPE_CODEBUDDY,
    key: 'test-key',
    models: 'claude-sonnet-4',
    ...overrides,
  }
}

describe('CodeBuddy channel', () => {
  test('registers the channel type, ordering, model discovery, and icon metadata', () => {
    expect(CHANNEL_TYPE_CODEBUDDY).toBe(93)
    expect(CHANNEL_TYPES[CHANNEL_TYPE_CODEBUDDY]).toBe('CodeBuddy')
    expect(
      CHANNEL_TYPE_OPTIONS.find(
        (item) => item.value === CHANNEL_TYPE_CODEBUDDY
      )
    ).toStrictEqual({ value: CHANNEL_TYPE_CODEBUDDY, label: 'CodeBuddy' })
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_CODEBUDDY)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_CODEBUDDY)).toBe('CodeBuddy')
    expect(
      CHANNEL_PROVIDER_PRESENTATION[CHANNEL_TYPE_CODEBUDDY]?.descriptionKey
    ).toBe('Connect to CodeBuddy model services')
  })

  test('declares no default base URL', () => {
    expect(
      CHANNEL_TYPE_DEFAULTS[CHANNEL_TYPE_CODEBUDDY]?.base_url
    ).toBeUndefined()
  })
})

describe('CodeBuddy access mode', () => {
  test('defaults the region marker to empty', () => {
    expect(CHANNEL_FORM_DEFAULT_VALUES.codebuddy_region).toBe('')
  })

  test('accepts cn, intl and empty in the form schema', () => {
    expect(
      channelFormSchema.safeParse(codebuddyForm({ codebuddy_region: 'cn' }))
        .success
    ).toBe(true)
    expect(
      channelFormSchema.safeParse(codebuddyForm({ codebuddy_region: 'intl' }))
        .success
    ).toBe(true)
    expect(
      channelFormSchema.safeParse(codebuddyForm({ codebuddy_region: '' }))
        .success
    ).toBe(true)
    expect(
      channelFormSchema.safeParse(codebuddyForm({ codebuddy_region: 'eu' }))
        .success
    ).toBe(false)
  })

  test('writes the region marker into settings only for type 93', () => {
    const payload = transformFormDataToCreatePayload(
      codebuddyForm({ codebuddy_region: 'intl' })
    )
    expect(
      JSON.parse(payload.channel.settings as string).codebuddy_region
    ).toBe('intl')

    const emptyPayload = transformFormDataToCreatePayload(
      codebuddyForm({ codebuddy_region: '' })
    )
    expect(
      'codebuddy_region' in JSON.parse(emptyPayload.channel.settings as string)
    ).toBe(false)

    const otherType = transformFormDataToCreatePayload(
      codebuddyForm({ type: 94, codebuddy_region: 'intl' })
    )
    expect(
      'codebuddy_region' in JSON.parse(otherType.channel.settings as string)
    ).toBe(false)
  })

  test('round-trips the region marker and falls back to empty for unknown values', () => {
    const channel = channelSchema.parse({
      id: 1,
      type: CHANNEL_TYPE_CODEBUDDY,
      key: 'test-key',
      status: 1,
      name: 'CodeBuddy upstream',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'claude-sonnet-4',
      base_url: '',
      settings: JSON.stringify({ codebuddy_region: 'cn' }),
    })
    expect(transformChannelToFormDefaults(channel).codebuddy_region).toBe('cn')

    const unknown = channelSchema.parse({
      ...channel,
      settings: JSON.stringify({ codebuddy_region: 'eu' }),
    })
    expect(transformChannelToFormDefaults(unknown).codebuddy_region).toBe('')
  })
})
