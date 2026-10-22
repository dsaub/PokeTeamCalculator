// Preload de `bun test`: entorno de navegador con happy-dom para los tests de componentes.
import { GlobalRegistrator } from '@happy-dom/global-registrator'

// happy-dom trae su propio fetch con política de mismo origen; los tests usan el de Bun
// (sustituido por dobles en `test/fetch.ts`, o real contra el servidor mock).
const nativos = {
  fetch: globalThis.fetch,
  Request: globalThis.Request,
  Response: globalThis.Response,
  Headers: globalThis.Headers,
}

GlobalRegistrator.register({ url: 'http://localhost/' })
Object.assign(globalThis, nativos)

const { afterEach } = await import('bun:test')
const { cleanup } = await import('@testing-library/react')
const { restaurarDobles } = await import('./fetch.ts')

afterEach(() => {
  cleanup()
  restaurarDobles()
  localStorage.clear()
})
