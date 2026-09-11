package routing

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestSubtractAll(t *testing.T) {
	bases := mustPrefixes(t, "10.0.0.0/24")
	exclusions := mustPrefixes(t, "10.0.0.64/26")
	got := prefixStrings(SubtractAll(bases, exclusions))
	want := []string{"10.0.0.0/26", "10.0.0.128/25"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SubtractAll() = %v, want %v", got, want)
	}
}

func TestSubtractAllRemovesWholeNetwork(t *testing.T) {
	bases := mustPrefixes(t, "10.0.0.0/24")
	exclusions := mustPrefixes(t, "10.0.0.0/16")
	if got := SubtractAll(bases, exclusions); len(got) != 0 {
		t.Fatalf("SubtractAll() = %v, want empty result", got)
	}
}

func TestNormalizeRemovesCoveredPrefixes(t *testing.T) {
	got := prefixStrings(Normalize(mustPrefixes(t, "10.0.0.0/8", "10.1.0.0/16", "192.0.2.0/24")))
	want := []string{"10.0.0.0/8", "192.0.2.0/24"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %v, want %v", got, want)
	}
}

func mustPrefixes(t *testing.T, values ...string) []netip.Prefix {
	t.Helper()
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, prefix)
	}
	return result
}

func prefixStrings(prefixes []netip.Prefix) []string {
	result := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		result = append(result, prefix.String())
	}
	return result
}
