package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	PEER_REGEX = regexp.MustCompile(`(tcp|tls)://([a-z0-9\.\-\:\[\]]+):([0-9]+)`)
)

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
		log.Printf("Resolve error %T: %s", err, err)
		return
	}

	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", "["+addr+"]:"+strconv.Itoa(peer.port), 5*time.Second)
	if err != nil {
		log.Printf("Connection error %T: %s", err, err)
		return
	}
	defer conn.Close()

	peer.Latency = time.Since(startTime)
	peer.Up = true
}

func printResults(results []Peer) {
	fmt.Println("Dead peers:")
	for _, p := range results {
		if !p.Up {
			fmt.Printf("%s\t%s/%s\n", p.URI, p.Region, p.Country)
		}
	}

	fmt.Println("\n\nAlive peers (sorted by latency):")
	fmt.Println("URI\tLatency (ms)\tLocation")
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
		fmt.Printf("%s\t%.3f\t%s/%s\n", p.URI, latency, p.Region, p.Country)
	}
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
