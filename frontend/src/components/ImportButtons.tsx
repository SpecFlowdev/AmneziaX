import { useMemo, useState } from 'react'
import { Icon } from './icons'
import { useI18n } from '../i18n'

/**
 * One-tap import into a client app.
 *
 * Every entry is a documented custom URL scheme the app registers with the OS.
 * The panel cannot know whether an app is installed — the browser simply hands
 * the URL over and nothing visible happens if nobody claims it — so the copy
 * link and the QR stay on the page as the route that always works.
 */

type Platform = 'ios' | 'android' | 'windows' | 'macos' | 'linux'

interface App {
  name: string
  /** Builds the scheme URL from the subscription link. */
  href: (sub: string) => string
  platforms: Platform[]
}

const APPS: App[] = [
  {
    name: 'Happ',
    href: (s) => `happ://add/${s}`,
    platforms: ['ios', 'android', 'macos', 'windows'],
  },
  {
    name: 'v2rayNG',
    href: (s) => `v2rayng://install-sub?url=${encodeURIComponent(s)}`,
    platforms: ['android'],
  },
  {
    name: 'Streisand',
    href: (s) => `streisand://import/${s}`,
    platforms: ['ios', 'macos'],
  },
  {
    name: 'Hiddify',
    href: (s) => `hiddify://install-sub?url=${encodeURIComponent(s)}`,
    platforms: ['android', 'ios', 'windows', 'macos', 'linux'],
  },
  {
    name: 'Shadowrocket',
    // Shadowrocket takes a sub:// payload, which is the link in base64.
    href: (s) => `shadowrocket://add/sub://${b64url(s)}`,
    platforms: ['ios'],
  },
  {
    name: 'sing-box',
    href: (s) => `sing-box://import-remote-profile?url=${encodeURIComponent(s)}`,
    platforms: ['ios', 'android', 'windows', 'macos', 'linux'],
  },
  {
    name: 'Clash',
    href: (s) => `clash://install-config?url=${encodeURIComponent(s)}`,
    platforms: ['windows', 'macos', 'linux', 'android'],
  },
  {
    name: 'V2Box',
    href: (s) => `v2box://install-sub?url=${encodeURIComponent(s)}`,
    platforms: ['ios', 'android'],
  },
]

function b64url(s: string): string {
  // btoa only handles latin1; a subscription URL is ASCII, but encode defensively.
  return btoa(unescape(encodeURIComponent(s)))
}

/** detectPlatform reads the user agent, which is a hint and not a fact. */
export function detectPlatform(ua: string): Platform | null {
  const s = ua.toLowerCase()
  // iPadOS reports itself as a Mac, so the touch check has to come first.
  if (/iphone|ipad|ipod/.test(s)) return 'ios'
  if (/android/.test(s)) return 'android'
  if (/windows/.test(s)) return 'windows'
  if (/mac os x|macintosh/.test(s)) return 'macos'
  if (/linux|x11/.test(s)) return 'linux'
  return null
}

export function ImportButtons({ subscriptionUrl }: { subscriptionUrl: string }) {
  const { t } = useI18n()
  const [showAll, setShowAll] = useState(false)

  const platform = useMemo(
    () =>
      typeof navigator === 'undefined'
        ? null
        : detectPlatform(
            navigator.userAgent +
              // iPadOS masquerades as macOS; a touch-capable "Mac" is an iPad.
              (navigator.maxTouchPoints > 1 ? ' ipad' : ''),
          ),
    [],
  )

  const forHere = platform ? APPS.filter((a) => a.platforms.includes(platform)) : []
  const rest = APPS.filter((a) => !forHere.includes(a))
  const shown = showAll || forHere.length === 0 ? APPS : forHere

  return (
    <div className="stack" style={{ gap: 8, width: '100%', alignItems: 'center' }}>
      <span className="small muted">{t.sub.openIn}</span>
      <div className="split" style={{ gap: 8, flexWrap: 'wrap', justifyContent: 'center' }}>
        {shown.map((app) => (
          <a
            key={app.name}
            className="pill"
            href={app.href(subscriptionUrl)}
            // These leave the page for another application, so no target/_blank:
            // a new tab that never navigates just leaves a blank window behind.
            rel="noreferrer"
          >
            <Icon name="download" size={13} />
            {app.name}
          </a>
        ))}
      </div>
      {!showAll && forHere.length > 0 && rest.length > 0 && (
        <button className="btn-ghost small" onClick={() => setShowAll(true)}>
          {t.sub.otherApps}
        </button>
      )}
      <span className="small dim" style={{ textAlign: 'center' }}>
        {t.sub.openInHint}
      </span>
    </div>
  )
}
