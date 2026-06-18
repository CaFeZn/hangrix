import { tryRecoverChunkFailure } from '../utils/chunk-recovery'

export default defineNuxtPlugin((nuxtApp) => {
  window.addEventListener('pageshow', () => {
    try {
      sessionStorage.removeItem('__hangrix_chunk_retry')
    } catch {}
  })

  nuxtApp.hook('app:error', (error) => {
    if (tryRecoverChunkFailure(error)) {
      return
    }
  })

  const router = useRouter()
  router.onError((error) => {
    tryRecoverChunkFailure(error)
  })
})
