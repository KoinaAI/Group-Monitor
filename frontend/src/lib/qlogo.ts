// QQ public avatar (qlogo) URL helpers. These hit Tencent's CDN directly from
// the browser — the Go backend does not proxy them — so every avatar has an
// <Avatar.Fallback> for the offline / private-profile case. Keep sizes to the
// documented steps (40/100/140/640); other values 404 to a default image.

// Group avatar. `p.qlogo.cn/gh/{gid}/{gid}/{size}/` is the stable public form.
export function groupAvatar(groupId: number | string, size = 100): string {
  return `https://p.qlogo.cn/gh/${groupId}/${groupId}/${size}/`
}

// User avatar by QQ number. `s=0` returns the largest available; we pin a size
// so the browser can cache one variant across the app.
export function userAvatar(userId: number | string, size = 100): string {
  return `https://q1.qlogo.cn/g?b=qq&nk=${userId}&s=${size}`
}
