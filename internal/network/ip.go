package network

import (
	"errors"
	"net"
)

// DiscoverLANIP finds a suitable IPv4 LAN address.
// If interfaceName is non-empty, only that interface is considered.
func DiscoverLANIP(interfaceName string) (net.IP, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifs {
		if interfaceName != "" && iface.Name != interfaceName {
			continue
		}
		// Skip down or loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue // skip IPv6
			}
			if isPrivateIPv4(ip4) {
				return ip4, nil
			}
		}
	}
	return nil, errors.New("no suitable LAN IPv4 address found")
}

func isPrivateIPv4(ip net.IP) bool {
	// 10.0.0.0/8
	if ip[0] == 10 {
		return true
	}
	// 172.16.0.0/12
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}
	// 192.168.0.0/16
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}
	return false
}
