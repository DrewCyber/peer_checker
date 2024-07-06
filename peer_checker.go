package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/quic-go/quic-go"
)

var (
	PEER_REGEX = regexp.MustCompile(`(tcp|tls|quic)://([a-z0-9\.\-\:\[\]]+):([0-9]+)`)
)

const connTimeout = 5 * time.Second

type Peer struct {
	URI      string
	protocol string
	host     string
	port     int
	Region   string
	Country  string
	Up       bool
	Latency  time.Duration
}

func getPeers(dataDir string, regions []string, countries []string) ([]Peer, error) {
	peers := []Peer{}
	allRegions, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}

	allCountries := []string{}
	for _, region := range allRegions {
		if region.Name() != ".git" && region.Name() != "other" && region.IsDir() {
			countries, err := os.ReadDir(dataDir + "/" + region.Name())
			if err != nil {
				return nil, err
			}
			for _, country := range countries {
				if strings.HasSuffix(country.Name(), ".md") {
					allCountries = append(allCountries, country.Name())
				}
			}
		}
	}

	if len(regions) == 0 {
		regions = make([]string, len(allRegions))
		for i, region := range allRegions {
			regions[i] = region.Name()
		}
	}
	if len(countries) == 0 {
		countries = allCountries
	}

	for _, region := range regions {
		for _, country := range countries {
			cfile := fmt.Sprintf("%s/%s/%s", dataDir, region, country)
			if _, err := os.Stat(cfile); err == nil {
				content, err := os.ReadFile(cfile)
				if err != nil {
					return nil, err
				}
				matches := PEER_REGEX.FindAllStringSubmatch(string(content), -1)
				for _, match := range matches {
					// fmt.Println("Match:", match)
					uri := match[0]
					protocol := match[1]
					host := match[2]
					port, _ := strconv.Atoi(match[3])
					peers = append(peers, Peer{
						URI:      uri,
						protocol: protocol,
						host:     host,
						port:     port,
						Region:   region,
						Country:  country,
					})
				}
			}
		}
	}

	return peers, nil
}

func resolve(name string) (string, error) {
	if strings.HasPrefix(name, "[") {
		return name[1 : len(name)-1], nil
	}

	ips, err := net.LookupIP(name)
	if err != nil {
		return "", err
	}
	return ips[0].String(), nil
}

func isUp(peer *Peer) {
	addr, err := resolve(peer.host)
	if err != nil {
		slog.Debug("Resolve error:", "msg", err, "type", fmt.Sprintf("%T", err))
		return
	}

	switch peer.protocol {
	case "tcp", "tls":
		startTime := time.Now()
		// Dial the TCP/TLS server
		conn, err := net.DialTimeout("tcp", "["+addr+"]:"+strconv.Itoa(peer.port), connTimeout)
		if err != nil {
			slog.Debug("Connection error:", "msg", err, "type", fmt.Sprintf("%T", err))
			return
		}
		defer conn.Close()
		peer.Latency = time.Since(startTime)
		peer.Up = true
	case "quic":
		// Create a context
		ctx := context.Background()

		// Dial the QUIC server
		startTime := time.Now()
		conn, err := quic.DialAddr(ctx, "["+addr+"]:"+strconv.Itoa(peer.port), &tls.Config{InsecureSkipVerify: true}, nil)
		if err != nil {
			slog.Debug("Connection error:", "msg", err, "type", fmt.Sprintf("%T", err))
			return
		}
		defer conn.CloseWithError(0, "Closing connection")
		peer.Latency = time.Since(startTime)
		peer.Up = true
	}
}

func printResults(results []Peer) {
	fmt.Println("Dead peers:")
	deadTable := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(deadTable, "URI\tLocation")
	for _, p := range results {
		if !p.Up {
			fmt.Fprintf(deadTable, "%s\t%s/%s\n", p.URI, p.Region, p.Country)
		}
	}
	deadTable.Flush()

	fmt.Println("\n\nAlive peers (sorted by latency):")
	aliveTable := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(aliveTable, "URI\tLatency (ms)\tLocation")
	alivePeers := []Peer{}
	for _, p := range results {
		if p.Up {
			alivePeers = append(alivePeers, p)
		}
	}
	sort.Slice(alivePeers, func(i, j int) bool {
		return alivePeers[i].Latency < alivePeers[j].Latency
	})
	for _, p := range alivePeers {
		latency := p.Latency.Seconds() * 1000
		fmt.Fprintf(aliveTable, "%s\t%.3f\t%s/%s\n", p.URI, latency, p.Region, p.Country)
	}
	aliveTable.Flush()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s [path to public_peers repository on a disk]\n", os.Args[0])
		fmt.Printf("I.e.:  %s ~/Projects/yggdrasil/public_peers\n", os.Args[0])
		return
	}

	dataDir := os.Args[1]

	peers, err := getPeers(dataDir, nil, nil)
	if err != nil {
		fmt.Printf("Can't find peers in a directory: %s\n", dataDir)
		return
	}

	fmt.Println("Report date:", time.Now().Format(time.RFC1123))

	var wg sync.WaitGroup

	for i := range peers {
		wg.Add(1)
		go func(p *Peer) {
			defer wg.Done()
			isUp(p)
		}(&peers[i])
	}

	wg.Wait()
	printResults(peers)
}
