package network

import (
	"fmt"
	"net"
)

// DiscoverLANIP finds a suitable IPv4 LAN address using a scoring system.
// It prioritizes Private > Public > Loopback > Link-Local.
// Interface status (Up) boosts the score but is not a hard requirement.
// If interfaceName is specified but not found, returns an error.
// If no suitable IP is found, it falls back to the loopback address (127.0.0.1).
func DiscoverLANIP(interfaceName string) (net.IP, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	var bestIP net.IP
	bestScore := -1
	foundInterface := false

	for _, iface := range ifs {
		if interfaceName != "" {
			if iface.Name == interfaceName {
				foundInterface = true
			} else {
				continue
			}
		}

		// Calculate interface score bonus
		ifaceScore := 0
		if iface.Flags&net.FlagUp != 0 {
			ifaceScore += 10
		} else {
			ifaceScore -= 10
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
				continue // Skip IPv6
			}

			// Base score based on IP type
			score := 0
			isLinkLocal := ip4[0] == 169 && ip4[1] == 254

			if isPrivateIPv4(ip4) {
				score = 100
			} else if ip4.IsLoopback() {
				score = 20
			} else if isLinkLocal {
				score = 1
			} else {
				score = 50 // Public or other routable IP
			}

			// Add interface bonuses
			finalScore := score + ifaceScore

			if finalScore > bestScore {
				bestScore = finalScore
				bestIP = ip4
			}
		}
	}

	// If a specific interface was requested but not found, return error
	if interfaceName != "" && !foundInterface {
		return nil, net.UnknownNetworkError("interface not found: " + interfaceName)
	}

	if bestIP != nil {
		return bestIP, nil
	}

	// Fallback to loopback if absolutely nothing else is found
	return net.ParseIP("127.0.0.1"), nil
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
