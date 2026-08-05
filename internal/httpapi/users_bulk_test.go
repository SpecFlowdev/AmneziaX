package httpapi

import (
	"strings"
	"testing"
)

func TestBulkUsernamesFromPrefix(t *testing.T) {
	got, err := bulkCreateRequest{Prefix: "team-", Count: 3}.usernames()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"team-1", "team-2", "team-3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Names have to sort the way they read. Without a fixed width, user-10 sorts
// before user-2 in every list the operator will ever look at.
func TestBulkUsernamesPadToEqualWidth(t *testing.T) {
	got, err := bulkCreateRequest{Prefix: "u", Count: 11}.usernames()
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "u01" || got[9] != "u10" || got[10] != "u11" {
		t.Fatalf("padding is wrong: %v", got)
	}

	// Width comes from the highest number, not the count, so a run starting at
	// 95 and ending at 104 pads to three everywhere.
	got, err = bulkCreateRequest{Prefix: "u", Count: 10, Start: 95}.usernames()
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "u095" || got[len(got)-1] != "u104" {
		t.Fatalf("start offset padding is wrong: first=%s last=%s", got[0], got[len(got)-1])
	}
}

func TestBulkUsernamesFromListWins(t *testing.T) {
	got, err := bulkCreateRequest{
		Prefix: "ignored-", Count: 99,
		Names: []string{" alice ", "bob", "", "alice", "ALICE", "carol"},
	}.usernames()
	if err != nil {
		t.Fatal(err)
	}
	// Trimmed, blanks dropped, and repeats folded case-insensitively — a pasted
	// list is exactly where those three show up.
	want := "alice,bob,carol"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}

func TestBulkUsernamesRejectsNonsense(t *testing.T) {
	cases := map[string]bulkCreateRequest{
		"no prefix and no names": {Count: 5},
		"zero count":             {Prefix: "u", Count: 0},
		"negative count":         {Prefix: "u", Count: -3},
		"over the cap":           {Prefix: "u", Count: maxBulkCreate + 1},
		"list over the cap":      {Names: make([]string, 0)},
	}
	for name, req := range cases {
		if name == "list over the cap" {
			for i := 0; i <= maxBulkCreate; i++ {
				req.Names = append(req.Names, "user"+strings.Repeat("x", i%3)+string(rune('a'+i%26))+itoa(i))
			}
		}
		if _, err := req.usernames(); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestBulkUsernamesAtTheCapIsAllowed(t *testing.T) {
	got, err := bulkCreateRequest{Prefix: "u", Count: maxBulkCreate}.usernames()
	if err != nil {
		t.Fatalf("the cap itself must be allowed: %v", err)
	}
	if len(got) != maxBulkCreate {
		t.Fatalf("got %d names, want %d", len(got), maxBulkCreate)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
