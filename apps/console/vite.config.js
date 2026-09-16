import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const tlsDir = path.join(root, 'deploy', 'tls')
const tlsKey = path.join(tlsDir, 'gateway.key')
const tlsCert = path.join(tlsDir, 'gateway.crt')
// Dev: enable https when certs exist. Docker `vite build` has no certs → http (nginx serves static).
const https =
  fs.existsSync(tlsKey) && fs.existsSync(tlsCert)
    ? { key: fs.readFileSync(tlsKey), cert: fs.readFileSync(tlsCert) }
    : undefined

/**
 * guacamole-common-js@1.5.0 (latest on npm) has GUACAMOLE-1793: Blob ctor path
 * calls parseBlob(recordingBlob) without assigning recordingBlob = source, so
 * playback throws "Cannot read properties of undefined (reading 'size')".
 */
function fixGuacamoleSessionRecordingBlob() {
  return {
    name: 'fix-guacamole-session-recording-blob',
    transform(code, id) {
      if (!id.includes('guacamole-common')) return null
      let next = code
      // ESM / pretty: assign before parseBlob and keep a block.
      next = next.replace(
        /if\s*\(\s*source\s+instanceof\s+Blob\s*\)\s*\r?\n\s*parseBlob\(\s*recordingBlob\s*,\s*loadInstruction\s*,\s*notifyLoaded\s*\);/,
        'if (source instanceof Blob) {\n        recordingBlob = source;\n        parseBlob(recordingBlob, loadInstruction, notifyLoaded);\n    }'
      )
      // Minified: if(e instanceof Blob)g(t,w,S) → if(e instanceof Blob){t=e;g(t,w,S)}
      next = next.replace(
        /if\((\w+) instanceof Blob\)(\w+)\((\w+),(\w+),(\w+)\)/,
        'if($1 instanceof Blob){$3=$1;$2($3,$4,$5)}'
      )
      return next === code ? null : next
    }
  }
}

const controlApi = (process.env.OPS_CONTROL_INTERNAL_HTTP || 'http://127.0.0.1:9100').replace(/\/$/, '')
const gatewayPublic = (process.env.OPS_GATEWAY_PUBLIC_HTTP || 'https://127.0.0.1:9200').replace(/\/$/, '')
// Same as deploy/nginx.conf: browser WS is rewritten to Console origin, then proxied to Gateway INTERNAL.
const gatewayInternal = (process.env.OPS_GATEWAY_INTERNAL_HTTP || 'http://127.0.0.1:9201').replace(/\/$/, '')
/** Public Console origin for API Token docs (paths are under /api). Empty → browser location.origin at runtime. */
const consolePublicHttp = (process.env.OPS_CONSOLE_PUBLIC_HTTP || '').replace(/\/$/, '')

export default defineConfig({
  plugins: [vue(), fixGuacamoleSessionRecordingBlob()],
  define: {
    __WOOPS_CONSOLE_PUBLIC_HTTP__: JSON.stringify(consolePublicHttp)
  },
  // Avoid colliding with Vue route `/assets` (nginx would 301 to the real build dir).
  build: {
    assetsDir: 'static'
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    https,
    proxy: {
      '/api': controlApi,
      '/ws': {
        target: gatewayInternal,
        ws: true,
        changeOrigin: true
      },
      '/i': {
        // Same as deploy/nginx.conf: proxy to Gateway INTERNAL (plain HTTP).
        target: gatewayInternal,
        changeOrigin: true
      },
      '/bin': {
        target: gatewayInternal,
        changeOrigin: true
      }
    }
  }
})
