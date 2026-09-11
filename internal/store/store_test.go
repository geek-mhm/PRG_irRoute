package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestImportIranCIDRsNormalizesAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	content := "# data\n10.0.0.1\n10.0.0.1/32\n192.0.2.7/24 # normalized\n"
	if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	dataStore := New(NewPaths(root))
	count, hash, err := dataStore.ImportIranCIDRs(source)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(hash) != 64 {
		t.Fatalf("count = %d, hash length = %d", count, len(hash))
	}
	got, err := dataStore.LoadIranCIDRs()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.1/32", "192.0.2.0/24"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded data = %v, want %v", got, want)
	}
}
