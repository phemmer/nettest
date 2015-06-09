package main

import (
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/phemmer/gopacket/routing"
	sm "github.com/phemmer/sawmill"
	"github.com/phemmer/sawmill/handler/sentry"
	"github.com/phemmer/sawmill/handler/splunk"
	"golang.org/x/net/icmp"
	"golang.org/x/net/internal/iana"
	"golang.org/x/net/ipv4"
)

var localDNSAddr string = os.Getenv("LOCAL_DNS_ADDR")
var googleDNSAddr string = "8.8.8.8:53"
var gatewayPingAddr string = os.Getenv("GATEWAY_PING_ADDR")
var googlePingAddr string = "8.8.8.8"

func init() {
	if localDNSAddr == "" {
		cc, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil {
			sm.Fatal("error loading /etc/resolv.conf", sm.Fields{"error": err})
		}
		localDNSAddr = cc.Servers[0] + ":53"
	}

	if gatewayPingAddr == "" {
		router, err := routing.New()
		if err != nil {
			sm.Fatal("could not get IP router", sm.Fields{"error": err})
		}
		_, gatewayAddr, _, err := router.Route(net.ParseIP("8.8.8.8"))
		if err != nil {
			sm.Fatal("unable to get default route", sm.Fields{"error": err})
		}
		gatewayPingAddr = gatewayAddr.String()
	}
}

type Stats struct {
	msi map[string]interface{}
	sync.Mutex
}

func (s *Stats) Set(key string, value interface{}) {
	s.Lock()
	s.msi[key] = value
	s.Unlock()
}

func main() {
	defer sm.Stop()

	sentryDSN := os.Getenv("SENTRY_DSN")
	if sentryDSN != "" {
		sentryHandler, err := sentry.New(sentryDSN)
		if err != nil {
			sm.Error("Unable to initialize sentry handler", sm.Fields{"error": err})
		} else {
			sm.SetStackMinLevel(sm.ErrorLevel)
			sm.AddHandler("sentry", sm.FilterHandler(sentryHandler).LevelMin(sm.ErrorLevel))
		}
	}

	splunkURL := os.Getenv("SPLUNK_URL")
	if splunkURL != "" {
		splunkHandler, err := splunk.New(splunkURL)
		if err != nil {
			sm.Error("Unable to initialize splunk handler", sm.Fields{"error": err})
		} else {
			sm.AddHandler("splunk", sm.FilterHandler(splunkHandler).LevelMin(sm.InfoLevel))
		}
	}

	// this is just so elastic beanstalk can health check us
	noopHandler := func(w http.ResponseWriter, r *http.Request) {}
	go http.ListenAndServe("0.0.0.0:8080", http.HandlerFunc(noopHandler))

	wg := sync.WaitGroup{}
	ticker := time.NewTicker(time.Second * 5)
	for {
		stats := Stats{msi: map[string]interface{}{}}

		wg.Add(4)
		go checkResolve("local", localDNSAddr, &wg, &stats)
		go checkResolve("google", googleDNSAddr, &wg, &stats)
		go checkPing("gateway", gatewayPingAddr, &wg, &stats)
		go checkPing("google", googlePingAddr, &wg, &stats)

		wg.Wait()

		sm.Info("stats", stats.msi)

		<-ticker.C
	}
}

func checkResolve(host string, addr string, wg *sync.WaitGroup, stats *Stats) {
	defer wg.Done()

	timeStart := time.Now()

	m := &dns.Msg{}
	m.SetQuestion(dns.Fqdn("google.com"), dns.TypeA)
	_, err := dns.Exchange(m, addr)
	if err != nil {
		sm.Error("error performing lookup", sm.Fields{"error": err, "host": host, "addr": addr})
		return
	}

	duration := time.Now().Sub(timeStart)

	stats.Set("resolve."+host+".time", float64(duration.Nanoseconds())/float64(time.Millisecond))
}

func checkPing(host string, addr string, wg *sync.WaitGroup, stats *Stats) {
	defer wg.Done()

	timeStart := time.Now()

	//c, err := icmp.ListenPacket("udp4", "0.0.0.0")
	c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		sm.Error("unable to listen for udp", sm.Fields{"error": err})
		return
	}
	defer c.Close()

	m := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   1,
			Seq:  1,
			Data: []byte("foo bar"),
		},
	}
	mm, err := m.Marshal(nil)
	if err != nil {
		sm.Error("unable to marshal message", sm.Fields{"error": err})
		return
	}

	c.SetDeadline(time.Now().Add(time.Second))

	//if _, err := c.WriteTo(mm, &net.UDPAddr{IP: net.ParseIP(addr)}); err != nil {
	if _, err := c.WriteTo(mm, &net.IPAddr{IP: net.ParseIP(addr)}); err != nil {
		sm.Error("unable to send echo request", sm.Fields{"error": err})
		return
	}

	rb := make([]byte, 1500)
	n, _, err := c.ReadFrom(rb)
	if err != nil {
		sm.Error("unable to read response", sm.Fields{"error": err})
		return
	}

	if false {
		_, err := icmp.ParseMessage(iana.ProtocolIPv6ICMP, rb[:n])
		if err != nil {
			sm.Error("unable to parse response", sm.Fields{"error": err})
			return
		}
	}

	duration := time.Now().Sub(timeStart)

	stats.Set("ping."+host+".time", float64(duration.Nanoseconds())/float64(time.Millisecond))
}
