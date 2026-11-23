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

export const CHANNEL_EXPORT_PASSWORD_MIN = 8
export const CHANNEL_EXPORT_PASSWORD_MAX = 128

/**
 * The password length rule of the export file, as an i18n error key. Count
 * code points rather than UTF-16 code units so the check matches the backend's
 * rune count (4 emoji are 8 code units but 4 runes).
 */
export function validateChannelExportPassword(password: string): string | null {
  const length = [...password].length
  if (
    length < CHANNEL_EXPORT_PASSWORD_MIN ||
    length > CHANNEL_EXPORT_PASSWORD_MAX
  ) {
    return 'Password must contain between 8 and 128 characters.'
  }
  return null
}

/** Save the encrypted file the backend returned under the filename it chose. */
export function downloadChannelExport(
  filename: string,
  content: string
): void {
  const blob = new Blob([content], {
    type: 'application/json;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

/** Read a chosen export file as the text the import endpoint expects. */
export function readChannelExportFile(file: File): Promise<string> {
  return file.text()
}
