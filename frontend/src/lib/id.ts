// crypto.randomUUID() exists only in a secure context (HTTPS or localhost). This
// standalone SPA is frequently served over plain HTTP on a LAN IP, where it is
// undefined and would throw the moment a page mints an id (e.g. Setup calling
// newAccount() in a useState initializer). getRandomValues is available even in
// insecure contexts, so prefer it; fall back to Math.random only if both are gone.
export function uid(): string {
  const c = globalThis.crypto
  if (c?.randomUUID) return c.randomUUID()
  if (c?.getRandomValues) {
    const b = c.getRandomValues(new Uint8Array(16))
    b[6] = (b[6] & 0x0f) | 0x40 // version 4
    b[8] = (b[8] & 0x3f) | 0x80 // variant 10
    const h = [...b].map((x) => x.toString(16).padStart(2, '0'))
    return `${h.slice(0, 4).join('')}-${h.slice(4, 6).join('')}-${h.slice(6, 8).join('')}-${h.slice(8, 10).join('')}-${h.slice(10, 16).join('')}`
  }
  return `${Date.now().toString(16)}-${Math.random().toString(16).slice(2, 10)}`
}
