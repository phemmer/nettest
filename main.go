package main

import (
	//"net"
	"os"
	"sync"
	"time"

	"github.com/miekg/dns"
	sm "github.com/phemmer/sawmill"
	"github.com/phemmer/sawmill/handler/sentry"
	"github.com/phemmer/sawmill/handler/splunk"
)

var localDNSAddr string
var googleDNSAddr string = "8.8.8.8:53"

func init() {
	cc, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		sm.Fatal("error loading /etc/resolv.conf", sm.Fields{"error": err})
	}
	localDNSAddr = cc.Servers[0] + ":53"
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

	wg := sync.WaitGroup{}
	ticker := time.NewTicker(time.Second * 5)
	for {
		stats := Stats{msi: map[string]interface{}{}}

		wg.Add(4)
		go checkResolve("local", localDNSAddr, &wg, &stats)
		go checkResolve("google", googleDNSAddr, &wg, &stats)
		go checkPingGateway(&wg, &stats)
		go checkPingGoogle(&wg, &stats)

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

	stats.Set("resolve."+host+".time", duration)
}

func checkPingGateway(wg *sync.WaitGroup, stats *Stats) {
	defer wg.Done()
}
func checkPingGoogle(wg *sync.WaitGroup, stats *Stats) {
	defer wg.Done()
}
