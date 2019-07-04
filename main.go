package main

import (
	"bytes"
	"log"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	bgpNeighborState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bgp_neighbor_state",
		Help: "The state of the connection to a given BGP neighbor (1=idle,2=connect,3=active,4=opensent,5=openconfirm,6=established)",
	},
		[]string{
			"ip",
		})
)

var (
	bgpNeighborAcceptedPrefixes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bgp_neighbor_accepted_prefixes",
		Help: "The number of accepted prefixes for a given BGP neighbor",
	},
		[]string{
			"ip",
		})
)

var (
	bgpNeighborConnectionsEstablished = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bgp_neighbor_connections_established",
		Help: "The number of connections that have been established for a given BGP neighbor",
	}, []string{
		"ip",
	})
)

var (
	bgpNeighborConnectionsDropped = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bgp_neighbor_connections_dropped",
		Help: "The number of connections that have been dropped for a given BGP neighbor",
	},
		[]string{
			"ip",
		})
)

// BgpNeighbor : This represents a BGP Neighbor
type BgpNeighbor struct {
	IP                     net.IP
	State                  float64
	AcceptedPrefixes       float64
	ConnectionsEstablished float64
	ConnectionsDropped     float64
}

var bgpNeighbors []BgpNeighbor

var bgpNeighborRegex = regexp.MustCompile(`^BGP neighbor is ([\d.]+), .*$`)
var bgpStateRegex = regexp.MustCompile(`^\s+BGP state = (\w+), .*$`)
var bgpAcceptedPrefixesRegex = regexp.MustCompile(`^\s+(\d+) accepted prefixes\w*$`)
var bgpConnectionsEstablishedDroppedRegex = regexp.MustCompile(`^\s+Connections established (\d+); dropped (\d+)\w*$`)

func recordMetrics() {
	go func() {
		for {
			o, _ := getBgpNeighbors()
			parseBGP(o)

			for _, n := range bgpNeighbors {
				bgpNeighborState.With(prometheus.Labels{"ip": n.IP.String()}).Set(n.State)
				bgpNeighborAcceptedPrefixes.With(prometheus.Labels{"ip": n.IP.String()}).Set(n.AcceptedPrefixes)
				bgpNeighborConnectionsEstablished.With(prometheus.Labels{"ip": n.IP.String()}).Set(n.ConnectionsEstablished)
				bgpNeighborConnectionsDropped.With(prometheus.Labels{"ip": n.IP.String()}).Set(n.ConnectionsDropped)
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

func getBgpNeighbors() (stdout string, stderr string) {
	cmd := exec.Command("vtysh", "-c", "show ip bgp neighbors")
	var sout, serr bytes.Buffer
	cmd.Stdout = &sout
	cmd.Stderr = &serr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to execute vtysh command: %s\n", err)
	}
	stdout, stderr = string(sout.Bytes()), string(serr.Bytes())
	return
}

func parseBGP(s string) {
	var bgpNeigh *BgpNeighbor
	neigh := ""
	for _, line := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		check := bgpNeighborRegex.MatchString(line)
		if check {
			neigh = bgpNeighborRegex.FindStringSubmatch(line)[1]
			bgpNeigh = new(BgpNeighbor)
		}
		if neigh != "" {
			bgpNeigh.IP = net.ParseIP(neigh)

			checkState := bgpStateRegex.MatchString(line)
			if checkState {
				/* References from: https://github.com/troglobit/quagga/blob/master/bgpd/BGP4-MIB.txt
				Convert the state from string to int
				idle(1),
				connect(2),
				active(3),
				opensent(4),
				openconfirm(5),
				established(6)
				*/
				var state float64

				switch bgpStateRegex.FindStringSubmatch(line)[1] {
				case "Idle":
					state = 1
				case "Connect":
					state = 2
				case "Active":
					state = 3
				case "Opensent":
					state = 4
				case "Openconfirm":
					state = 5
				case "Established":
					state = 6
				}
				bgpNeigh.State = state
			}
			checkPrefixes := bgpAcceptedPrefixesRegex.MatchString(line)
			if checkPrefixes {
				pref, _ := strconv.ParseFloat(bgpAcceptedPrefixesRegex.FindStringSubmatch(line)[1], 64)
				bgpNeigh.AcceptedPrefixes = pref
			}
			checkConnections := bgpConnectionsEstablishedDroppedRegex.MatchString(line)
			if checkConnections {
				est, _ := strconv.ParseFloat(bgpConnectionsEstablishedDroppedRegex.FindStringSubmatch(line)[1], 64)
				drp, _ := strconv.ParseFloat(bgpConnectionsEstablishedDroppedRegex.FindStringSubmatch(line)[2], 64)
				bgpNeigh.ConnectionsEstablished = est
				bgpNeigh.ConnectionsDropped = drp

				var found bool = false
				for i := range bgpNeighbors {
					if bgpNeighbors[i].IP.String() == neigh {
						found = true
						bgpNeighbors[i] = *bgpNeigh
					}
				}
				if !found {
					bgpNeighbors = append(bgpNeighbors, *bgpNeigh)
				}
			}
		}
	}
}

func main() {
	prometheus.MustRegister(bgpNeighborState)
	prometheus.MustRegister(bgpNeighborAcceptedPrefixes)
	prometheus.MustRegister(bgpNeighborConnectionsEstablished)
	prometheus.MustRegister(bgpNeighborConnectionsDropped)

	recordMetrics()

	http.Handle("/metrics", promhttp.Handler())

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
             <head><title>BGP Exporter</title></head>
             <body>
             <h1>BGP Exporter</h1>
             <p><a href='/metrics'>Metrics</a></p>
             </body>
             </html>`))
	})

	http.ListenAndServe(":9114", nil)
}
