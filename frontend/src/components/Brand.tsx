import { useBranding } from '../lib/branding'
import { BrandMark } from './icons'

/**
 * The panel's identity: a custom logo when one is uploaded, the built-in mark
 * otherwise. The tagline only renders when an operator has actually set one.
 */
export function Brand({ size = 34, showTagline = true }: { size?: number; showTagline?: boolean }) {
  const { branding } = useBranding()

  return (
    <div className="split" style={{ gap: 11, minWidth: 0 }}>
      {branding.brandLogo ? (
        <img
          src={branding.brandLogo}
          alt=""
          width={size}
          height={size}
          style={{ width: size, height: size, borderRadius: 10, objectFit: 'cover', flex: 'none' }}
        />
      ) : (
        <div className="brand-mark" style={{ width: size, height: size }}>
          <BrandMark size={Math.round(size * 0.56)} />
        </div>
      )}
      <div className="stack" style={{ minWidth: 0 }}>
        <span className="brand-name truncate">{branding.brandName}</span>
        {showTagline && branding.brandTagline && (
          <span className="brand-sub truncate">{branding.brandTagline}</span>
        )}
      </div>
    </div>
  )
}
