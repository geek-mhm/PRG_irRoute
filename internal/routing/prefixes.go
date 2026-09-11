package routing

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

func ParseIPv4Prefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "/") {
		value += "/32"
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	if !prefix.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("only IPv4 prefixes are supported")
	}
	return prefix.Masked(), nil
}

func Normalize(prefixes []netip.Prefix) []netip.Prefix {
	seen := make(map[string]netip.Prefix)
	for _, prefix := range prefixes {
		prefix = prefix.Masked()
		seen[prefix.String()] = prefix
	}
	items := make([]netip.Prefix, 0, len(seen))
	for _, prefix := range seen {
		items = append(items, prefix)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Bits() != items[j].Bits() {
			return items[i].Bits() < items[j].Bits()
		}
		return items[i].Addr().Less(items[j].Addr())
	})

	result := make([]netip.Prefix, 0, len(items))
	for _, candidate := range items {
		covered := false
		for _, existing := range result {
			if existing.Contains(candidate.Addr()) {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, candidate)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Addr() != result[j].Addr() {
			return result[i].Addr().Less(result[j].Addr())
		}
		return result[i].Bits() < result[j].Bits()
	})
	return result
}

func SubtractAll(bases, exclusions []netip.Prefix) []netip.Prefix {
	result := Normalize(bases)
	for _, exclusion := range Normalize(exclusions) {
		next := make([]netip.Prefix, 0, len(result))
		for _, base := range result {
			next = append(next, subtract(base, exclusion)...)
		}
		result = next
	}
	return Normalize(result)
}

func subtract(base, exclusion netip.Prefix) []netip.Prefix {
	base = base.Masked()
	exclusion = exclusion.Masked()
	if !overlaps(base, exclusion) {
		return []netip.Prefix{base}
	}
	if exclusion.Bits() <= base.Bits() && exclusion.Contains(base.Addr()) {
		return nil
	}
	if base.Bits() >= 32 {
		return nil
	}
	left, right := split(base)
	result := subtract(left, exclusion)
	result = append(result, subtract(right, exclusion)...)
	return result
}

func overlaps(left, right netip.Prefix) bool {
	return left.Contains(right.Addr()) || right.Contains(left.Addr())
}

func split(prefix netip.Prefix) (netip.Prefix, netip.Prefix) {
	bits := prefix.Bits() + 1
	left := netip.PrefixFrom(prefix.Addr(), bits).Masked()
	address := prefix.Addr().As4()
	value := uint32(address[0])<<24 | uint32(address[1])<<16 | uint32(address[2])<<8 | uint32(address[3])
	value |= uint32(1) << (32 - bits)
	rightAddress := netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
	right := netip.PrefixFrom(rightAddress, bits)
	return left, right
}
