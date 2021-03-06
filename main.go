package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/alecthomas/template"
)

type Stat struct {
	DNSLookup        time.Duration
	TCPConnection    time.Duration
	TLSHandshake     time.Duration
	ServerProccesing time.Duration
	ContentTransfer  time.Duration
	Total            time.Duration
}

func main() {
	url := os.Args[1]
	stats := []Stat{}
	ts := os.Args[2]
	t, _ := strconv.Atoi(ts)
	var wg sync.WaitGroup
	for i := 0; i < t; i++ {
		wg.Add(1)
		go visit(url, &stats, &wg)
	}
	wg.Wait()
	sumarize(stats, os.Stdout)
}

func visit(url string, stats *[]Stat, wg *sync.WaitGroup) {
	var dnsStart, dnsDone, connDone, gotConn, transferInit, done time.Time
	defer wg.Done()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("new request failed: %v", err)
	}
	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:  func(_ httptrace.DNSDoneInfo) { dnsDone = time.Now() },
		ConnectStart: func(_, _ string) {
			if dnsDone.IsZero() {
				dnsDone = time.Now()
			}
		},
		ConnectDone: func(net, addr string, err error) {
			if err != nil {
				log.Fatalf("unable to connect to host %v: %v", addr, err)
			}
			connDone = time.Now()
		},
		GotConn:              func(_ httptrace.GotConnInfo) { gotConn = time.Now() },
		GotFirstResponseByte: func() { transferInit = time.Now() },
	}
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), trace))
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
	}
	_, err = client.Do(req)
	if err != nil {
		log.Fatalf("request failed: %v", err)
	}
	done = time.Now()
	if transferInit.IsZero() {
		transferInit = done
	}
	if dnsStart.IsZero() {
		dnsStart = dnsDone
	}
	stat := Stat{
		DNSLookup:        dnsDone.Sub(dnsStart),
		TCPConnection:    connDone.Sub(dnsDone),
		TLSHandshake:     gotConn.Sub(connDone),
		ServerProccesing: transferInit.Sub(gotConn),
		ContentTransfer:  done.Sub(transferInit),
		Total:            done.Sub(dnsStart),
	}
	iStats := *stats
	iStats = append(iStats, stat)
	*stats = iStats
}

func sumarize(stats []Stat, w io.Writer) {
	summ := Stat{}
	for _, s := range stats {
		summ.DNSLookup = time.Duration(summ.DNSLookup.Nanoseconds() + s.DNSLookup.Nanoseconds())
		summ.TCPConnection = time.Duration(summ.TCPConnection.Nanoseconds() + s.TCPConnection.Nanoseconds())
		summ.TLSHandshake = time.Duration(summ.TLSHandshake.Nanoseconds() + s.TLSHandshake.Nanoseconds())
		summ.ServerProccesing = time.Duration(summ.ServerProccesing.Nanoseconds() + s.ServerProccesing.Nanoseconds())
		summ.ContentTransfer = time.Duration(summ.ContentTransfer.Nanoseconds() + s.ContentTransfer.Nanoseconds())
		summ.Total = time.Duration(summ.Total.Nanoseconds() + s.Total.Nanoseconds())
	}
	summ.DNSLookup = time.Duration((summ.DNSLookup.Nanoseconds() / int64(len(stats))))
	summ.TCPConnection = time.Duration((summ.TCPConnection.Nanoseconds() / int64(len(stats))))
	summ.TLSHandshake = time.Duration((summ.TLSHandshake.Nanoseconds() / int64(len(stats))))
	summ.ServerProccesing = time.Duration((summ.ServerProccesing.Nanoseconds() / int64(len(stats))))
	summ.ContentTransfer = time.Duration((summ.ContentTransfer.Nanoseconds() / int64(len(stats))))
	summ.Total = time.Duration((summ.Total.Nanoseconds() / int64(len(stats))))
	sumaryTmpl := `Average request time: {{.Total.Seconds }}s
DNS Lookup: {{ .DNSLookup.Seconds }}s
TCP Connections: {{ .TCPConnection.Seconds }}s
Server Procesing: {{ .ServerProccesing.Seconds }}s
Server Tranfer: {{ .ContentTransfer.Seconds }}s
`
	tmpl, _ := template.New("summary").Parse(sumaryTmpl)
	tmpl.Execute(w, summ)
}
