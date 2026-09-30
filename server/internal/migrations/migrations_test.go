package migrations

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var versionPrefix = regexp.MustCompile(`^(\d+)_.+\.sql$`)

func TestMigrationFilesAreOrderedAndReversible(t *testing.T) {
	entries, err := files.ReadDir("sql")
	if err != nil {
		t.Fatal(err)
	}

	var highest int64
	for _, entry := range entries {
		match := versionPrefix.FindStringSubmatch(entry.Name())
		if match == nil {
			t.Fatalf("migration %q lacks a numeric version prefix", entry.Name())
		}
		version, _ := strconv.ParseInt(match[1], 10, 64)
		if version <= highest {
			t.Fatalf("migration %q is out of order", entry.Name())
		}
		highest = version

		body, err := files.ReadFile("sql/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !strings.Contains(text, "+goose Up") || !strings.Contains(text, "+goose Down") {
			t.Fatalf("migration %q must define Up and Down", entry.Name())
		}
	}

	if highest != Latest {
		t.Fatalf("Latest = %d, highest migration file = %d", Latest, highest)
	}
}
