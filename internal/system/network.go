package system

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Interface struct {
	Name      string
	MAC       string
	Addresses []string
}

type defaultRouteRecord struct {
	Gateway   string `json:"gateway"`
	Interface string `json:"dev"`
}

type addressRecord struct {
	Name    string `json:"ifname"`
	Address string `json:"address"`
	Info    []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

func DiscoverInterfaces(runner Runner) ([]Interface, error) {
	output, err := runner.Run("ip", "-j", "address", "show")
	if err != nil {
		return nil, err
	}
	var records []addressRecord
	if err := json.Unmarshal([]byte(output), &records); err != nil {
		return nil, fmt.Errorf("decode interface data: %w", err)
	}
	interfaces := make([]Interface, 0, len(records))
	for _, record := range records {
		if record.Name == "lo" {
			continue
		}
		item := Interface{Name: record.Name, MAC: record.Address}
		for _, address := range record.Info {
			if address.Family == "inet" {
				item.Addresses = append(item.Addresses, fmt.Sprintf("%s/%d", address.Local, address.PrefixLen))
			}
		}
		interfaces = append(interfaces, item)
	}
	return interfaces, nil
}

func DiscoverDefaultGateways(runner Runner) (map[string]string, error) {
	output, err := runner.Run("ip", "-j", "-4", "route", "show", "default")
	if err != nil {
		return nil, err
	}
	var records []defaultRouteRecord
	if err := json.Unmarshal([]byte(output), &records); err != nil {
		return nil, fmt.Errorf("decode default routes: %w", err)
	}
	result := make(map[string]string)
	for _, record := range records {
		if record.Interface != "" && record.Gateway != "" {
			result[record.Interface] = record.Gateway
		}
	}
	return result, nil
}

func InterfaceExists(runner Runner, name string) error {
	_, err := runner.Run("ip", "link", "show", "dev", name)
	return err
}

func InterfaceMAC(runner Runner, name string) (string, error) {
	output, err := runner.Run("ip", "-j", "link", "show", "dev", name)
	if err != nil {
		return "", err
	}
	var records []struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(output), &records); err != nil || len(records) != 1 {
		return "", fmt.Errorf("could not read MAC address for interface %s", name)
	}
	return strings.ToLower(records[0].Address), nil
}

func InterfaceHasAddress(runner Runner, name, address string) error {
	output, err := runner.Run("ip", "-o", "-4", "address", "show", "dev", name)
	if err != nil {
		return err
	}
	ip := strings.SplitN(address, "/", 2)[0]
	if !strings.Contains(output, " "+ip+"/") {
		return fmt.Errorf("interface %s does not have configured address %s", name, address)
	}
	return nil
}
