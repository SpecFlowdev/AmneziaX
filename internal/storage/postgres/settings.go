package postgres

import (
	"context"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

const settingsColumns = `brand_name, brand_tagline, brand_logo, brand_accent,
	subscription_title, support_url, currency, subscription_format,
	sub_page_show_links, sub_page_show_formats,
	clash_template, singbox_template, require_totp,
	warn_expiry_days, warn_quota_percent, updated_at`

func scanSettings(row interface{ Scan(...any) error }) (*domain.Settings, error) {
	var s domain.Settings
	err := row.Scan(&s.BrandName, &s.BrandTagline, &s.BrandLogo, &s.BrandAccent,
		&s.SubscriptionTitle, &s.SupportURL, &s.Currency, &s.SubscriptionFormat,
		&s.SubPageShowLinks, &s.SubPageShowFormats,
		&s.ClashTemplate, &s.SingBoxTemplate, &s.RequireTOTP,
		&s.WarnExpiryDays, &s.WarnQuotaPercent, &s.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	return &s, nil
}

// Settings returns the singleton settings row, creating it if a very old
// installation somehow lacks one.
func (s *Store) Settings(ctx context.Context) (*domain.Settings, error) {
	row := s.pool.QueryRow(ctx, `INSERT INTO panel_settings (id) VALUES (TRUE)
		ON CONFLICT (id) DO UPDATE SET id = TRUE
		RETURNING `+settingsColumns)
	return scanSettings(row)
}

func (s *Store) UpdateSettings(ctx context.Context, in domain.Settings) (*domain.Settings, error) {
	row := s.pool.QueryRow(ctx, `UPDATE panel_settings SET
		brand_name = $1, brand_tagline = $2, brand_logo = $3, brand_accent = $4,
		subscription_title = $5, support_url = $6, currency = $7,
		subscription_format = $8, sub_page_show_links = $9,
		sub_page_show_formats = $10, clash_template = $11,
		singbox_template = $12, require_totp = $13,
		warn_expiry_days = $14, warn_quota_percent = $15, updated_at = NOW()
		WHERE id = TRUE RETURNING `+settingsColumns,
		in.BrandName, in.BrandTagline, in.BrandLogo, in.BrandAccent,
		in.SubscriptionTitle, in.SupportURL, in.Currency, in.SubscriptionFormat,
		in.SubPageShowLinks, in.SubPageShowFormats,
		in.ClashTemplate, in.SingBoxTemplate, in.RequireTOTP,
		in.WarnExpiryDays, in.WarnQuotaPercent)
	return scanSettings(row)
}
