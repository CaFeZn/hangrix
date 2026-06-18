export function isChunkLoadFailure(error: unknown): boolean {
  const text = String(
    (error as { stack?: string; message?: string } | null)?.stack ||
      (error as { message?: string } | null)?.message ||
      error ||
      '',
  ).toLowerCase()

  return (
    text.includes('failed to fetch dynamically imported module') ||
    text.includes('importing a module script failed') ||
    text.includes('dynamically imported module') ||
    text.includes('failed to fetch')
  )
}

export function tryRecoverChunkFailure(error: unknown): boolean {
  if (!isChunkLoadFailure(error)) {
    return false
  }

  try {
    if (sessionStorage.getItem('__hangrix_chunk_retry') === '1') {
      return false
    }
    sessionStorage.setItem('__hangrix_chunk_retry', '1')
  } catch {
    return false
  }

  const url = new URL(window.location.href)
  url.searchParams.set('v', String(Date.now()))
  window.location.replace(url.toString())
  return true
}
