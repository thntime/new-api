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
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CodeBlock } from '@/components/ai-elements/code-block'
import { Button } from '@/components/ui/button'
import { tryPrettyJson } from '@/lib/utils'

import { getUpstreamPayload } from '../../api'
import { DetailSection } from './log-detail-layout'

const COLLAPSED_LINES = 12
const MAX_EXPANDED_LINES = 200

type PanelKind = 'request' | 'response'

function isJsonBody(text: string): boolean {
  try {
    JSON.parse(text)
    return true
  } catch {
    return false
  }
}

export function UpstreamPayloadPanels(props: { requestId: string }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<PanelKind | null>(null)
  const payloadQuery = useQuery({
    queryKey: ['usage-logs', 'upstream-payload', props.requestId],
    queryFn: () => getUpstreamPayload(props.requestId),
    enabled: expanded !== null && props.requestId !== '',
    staleTime: Infinity,
  })
  const payload = payloadQuery.data

  function renderBody(body: string | undefined, truncated: boolean) {
    if (payloadQuery.isLoading) {
      return (
        <p className='text-muted-foreground text-xs'>
          {t('Loading upstream payload...')}
        </p>
      )
    }
    if (payloadQuery.isError) {
      return (
        <p className='text-destructive text-xs'>
          {t('Failed to load the upstream payload.')}
        </p>
      )
    }
    if (payload === null) {
      return (
        <p className='text-muted-foreground text-xs'>
          {t('No upstream payload was recorded for this request.')}
        </p>
      )
    }
    if (!body) {
      return <p className='text-muted-foreground text-xs'>-</p>
    }
    const isJson = isJsonBody(body)
    return (
      <>
        {truncated && (
          <p className='text-xs text-amber-600 dark:text-amber-400'>
            {t('This payload was truncated at the capture size limit.')}
          </p>
        )}
        <CodeBlock
          code={isJson ? tryPrettyJson(body) : body}
          collapsedLines={COLLAPSED_LINES}
          defaultCollapsed
          language={isJson ? 'json' : 'plaintext'}
          maxExpandedLines={MAX_EXPANDED_LINES}
        />
      </>
    )
  }

  const renderPanel = (kind: PanelKind, title: string) => {
    const isOpen = expanded === kind
    let body: string | undefined
    let truncated = false
    if (payload != null) {
      if (kind === 'request') {
        body = payload.request_body
        truncated = payload.request_truncated
      } else {
        body = payload.response_body
        truncated = payload.response_truncated
      }
    }
    return (
      <DetailSection label={title}>
        <Button
          aria-expanded={isOpen}
          aria-label={`${isOpen ? t('Hide upstream payload') : t('Show upstream payload')} — ${title}`}
          className='h-6 w-full justify-start px-0 text-xs'
          onClick={() => setExpanded(isOpen ? null : kind)}
          size='sm'
          type='button'
          variant='ghost'
        >
          {isOpen ? (
            <ChevronDown className='size-3' aria-hidden='true' />
          ) : (
            <ChevronRight className='size-3' aria-hidden='true' />
          )}
          {isOpen ? t('Hide upstream payload') : t('Show upstream payload')}
        </Button>
        {isOpen && renderBody(body, truncated)}
      </DetailSection>
    )
  }

  return (
    <>
      {renderPanel('request', t('Upstream Request'))}
      {renderPanel('response', t('Upstream Response'))}
    </>
  )
}
