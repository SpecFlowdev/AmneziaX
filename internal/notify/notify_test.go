package notify

import (
	"strings"
	"testing"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// An empty subscription list means "everything". Getting this backwards
// creates a channel that looks configured, reports no errors, and delivers
// nothing — the worst failure mode a notification system has.
func TestChannelWants(t *testing.T) {
	for _, tc := range []struct {
		name    string
		channel domain.NotificationChannel
		kind    domain.EventKind
		want    bool
	}{
		{
			name:    "empty list takes everything",
			channel: domain.NotificationChannel{IsEnabled: true},
			kind:    domain.EventUserCreated,
			want:    true,
		},
		{
			name: "subscribed kind",
			channel: domain.NotificationChannel{IsEnabled: true,
				Events: []domain.EventKind{domain.EventUserCreated, domain.EventNodeError}},
			kind: domain.EventNodeError,
			want: true,
		},
		{
			name: "unsubscribed kind",
			channel: domain.NotificationChannel{IsEnabled: true,
				Events: []domain.EventKind{domain.EventUserCreated}},
			kind: domain.EventNodeError,
			want: false,
		},
		{
			name:    "disabled takes nothing, even with an empty list",
			channel: domain.NotificationChannel{IsEnabled: false},
			kind:    domain.EventUserCreated,
			want:    false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.channel.Wants(tc.kind); got != tc.want {
				t.Fatalf("Wants(%s) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

// Telegram parses the message as HTML, so a username is untrusted markup. A
// subscriber called `<b>x` must not be able to reformat an operator's alert,
// let alone break the message into an unparseable one.
func TestTelegramTextEscapesMarkup(t *testing.T) {
	out := TelegramText(domain.Event{
		Kind:    domain.EventUserCreated,
		Subject: `<b>evil</b> & "co"`,
		Message: "5 > 3 & <script>",
		Actor:   "a<dmin",
	})

	for _, raw := range []string{"<b>evil", "<script>", "a<dmin"} {
		if strings.Contains(out, raw) {
			t.Errorf("unescaped %q survived into the message:\n%s", raw, out)
		}
	}
	for _, want := range []string{"&lt;b&gt;evil&lt;/b&gt;", "&amp;", "&lt;script&gt;", "a&lt;dmin"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the message:\n%s", want, out)
		}
	}
}

func TestValidateWebhookURL(t *testing.T) {
	for _, tc := range []struct {
		url string
		ok  bool
	}{
		{"https://example.com/hook", true},
		{"http://127.0.0.1:9000/hook", true},

		// A scheme the HTTP client cannot speak, or none at all, is a
		// configuration mistake worth catching at save time rather than at
		// three in the morning when the first event fires.
		{"ftp://example.com/hook", false},
		{"file:///etc/passwd", false},
		{"example.com/hook", false},
		{"", false},
		{"https://", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			err := ValidateWebhookURL(tc.url)
			if tc.ok && err != nil {
				t.Fatalf("ValidateWebhookURL(%q) = %v, want accepted", tc.url, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("ValidateWebhookURL(%q) was accepted, want rejected", tc.url)
			}
		})
	}
}

// A channel kind the dispatcher does not know must not be retried three times
// before being written off.
func TestUnknownKindIsPermanent(t *testing.T) {
	if retryable(permanent{errStub{}}) {
		t.Fatal("a permanent failure was reported as retryable")
	}
	if !retryable(errStub{}) {
		t.Fatal("an ordinary failure was reported as permanent")
	}
}

type errStub struct{}

func (errStub) Error() string { return "stub" }
