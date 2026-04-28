package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Scanned struct {
	sync.Mutex
	Addresses map[string][]string
}

// LookupIPs resolves a domain name into a list of IP addresses.
//
// By default, it returns only IPv4 addresses. If includeIPv6 is true,
// it will also include IPv6 addresses in the result.
//
// If the domain cannot be resolved, it returns nil.
func LookupIPs(domain string, includeIPv6 bool) []string {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return nil
	}

	var res []string
	for _, ip := range ips {
		if ip.To4() != nil {
			res = append(res, ip.String())
		} else if includeIPv6 {
			res = append(res, ip.String())
		}
	}

	return res
}

// GetPresetPorts returns a list of ports based on a preset name.
//
// Available presets:
// - "common": common services
// - "web": web-related services
// - "dev": common development ports
func GetPresetPorts(name string) []int {
	switch name {
	case "common":
		return []int{22, 80, 443, 8080}
	case "web":
		return []int{80, 443, 8080, 8443}
	case "dev":
		return []int{3000, 5000, 5173, 8000, 8080}
	default:
		return nil
	}
}

// ParsePorts converts a comma-separated string like "22,80,443,8000-8005"
// into a slice of integers.
//
// Invalid values are skipped.
func ParsePorts(input string) []int {
	parts := strings.Split(input, ",")
	var ports []int

	for _, part := range parts {
		part = strings.TrimSpace(part)

		if strings.Contains(part, "-") {
			bounds := strings.Split(part, "-")
			if len(bounds) != 2 {
				continue
			}

			start, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || start > end {
				continue
			}

			for i := start; i <= end; i++ {
				ports = append(ports, i)
			}
			continue
		}

		port, err := strconv.Atoi(part)
		if err != nil {
			continue
		}

		ports = append(ports, port)
	}

	return ports
}

// ScanPort checks whether a TCP port is open on a given IP address.
//
// The result is stored safely inside the shared Scanned struct.
func ScanPort(ip string, port int, timeout int, sc *Scanned, wg *sync.WaitGroup) {
	defer wg.Done()

	address := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", address, time.Duration(timeout)*time.Millisecond)

	sc.Lock()
	defer sc.Unlock()

	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			sc.Addresses[ip] = append(sc.Addresses[ip], fmt.Sprintf("%d/tcp closed or filtered", port))
		} else {
			sc.Addresses[ip] = append(sc.Addresses[ip], fmt.Sprintf("%d/tcp closed", port))
		}
		return
	}

	conn.Close()
	sc.Addresses[ip] = append(sc.Addresses[ip], fmt.Sprintf("%d/tcp open", port))
}

func main() {
	target := flag.String("target", "", "Target IP address")
	openOnly := flag.Bool("open-only", false, "Open only exclude close ports, this could be usefull when running large input of ports")
	domain := flag.String("domain", "", "Target domain name")
	ports := flag.String("ports", "80,443", "Ports to scan, example: 22,80,443,8000-8005")
	preset := flag.String("preset", "", "Port preset: common, web, dev")
	timeout := flag.Int("timeout", 300, "Timeout in milliseconds")

	flag.Parse()

	var ipAddresses []string

	if *target != "" {
		ipAddresses = append(ipAddresses, *target)
	} else if *domain != "" {
		ipAddresses = LookupIPs(*domain, false)
		if len(ipAddresses) == 0 {
			log.Fatal("could not resolve domain")
		}
	} else {
		log.Fatal("target or domain must be provided")
	}

	var portsList []int

	if *preset != "" {
		portsList = GetPresetPorts(*preset)
		if portsList == nil {
			log.Fatal("invalid preset. Use: common, web, dev")
		}
	} else {
		portsList = ParsePorts(*ports)
	}

	if len(portsList) == 0 {
		log.Fatal("no valid ports provided")
	}

	sc := &Scanned{
		Addresses: make(map[string][]string),
	}

	start := time.Now()

	var wg sync.WaitGroup

	for _, ip := range ipAddresses {
		for _, port := range portsList {
			wg.Add(1)
			go ScanPort(ip, port, *timeout, sc, &wg)
		}
	}

	wg.Wait()

	fmt.Println("\nScan Summary")
	fmt.Println("------------")

	for _, ip := range ipAddresses {
		results := sc.Addresses[ip]
		sort.Strings(results)

		fmt.Printf("\n%s\n", ip)
		for _, result := range results {
			if *openOnly && strings.Contains(result, "closed") {
				continue
			}
			fmt.Printf("  - %s\n", result)
		}
	}

	fmt.Printf("\nScan completed in %s\n", time.Since(start))
}
