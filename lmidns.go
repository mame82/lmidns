//    This file is part of "Let Me In" for P4wnP1.
//
//    Copyright (c) 2018, Marcus Mengs.
//
//    P4wnP1 is free software: you can redistribute it and/or modify
//    it under the terms of the GNU General Public License as published by
//    the Free Software Foundation, either version 3 of the License, or
//    (at your option) any later version.
//
//    P4wnP1 is distributed in the hope that it will be useful,
//    but WITHOUT ANY WARRANTY; without even the implied warranty of
//    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//    GNU General Public License for more details.
//
//    You should have received a copy of the GNU General Public License
//    along with P4wnP1.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
	"log"
	"flag"
	"os"
	"strconv"
	"os/signal"
	"syscall"
)

var (
	pm = newPinMap()

	//flags
	domain = flag.String("domain",".","the domain to serve A records for")
	port = flag.Int("port", 53, "port to run on")


	reIP = regexp.MustCompile("(?m)(^[0-9]{1,3}-[0-9]{1,3}-[0-9]{1,3}-[0-9]{1,3})\\..*")
	//Note: case insensitive matching (nice observation: `ping` switches case on every attempt to resolve)
	reIPpin = regexp.MustCompile("(?mi)(^[0-9]{1,3}-[0-9]{1,3}-[0-9]{1,3}-[0-9]{1,3})-to-([0-9]{1,3}-[0-9]{1,3}-[0-9]{1,3}-[0-9]{1,3})-for-([0-9]{1,2})\\..*")
)

type pinMap struct {
	sync.Mutex
	m map[string]string
}

func newPinMap() (pm pinMap) {
	pm = pinMap{
		m: make(map[string]string),
	}
	return
}

// Note: if this function is called multiple times, the "delete after timeout" of
// the first call will trigger, anyway.
func (pm *pinMap) PinMapAdd(requestIP, responseIP string, timeout time.Duration) {
	go func() {
		pm.Lock()
		pm.m[requestIP] = responseIP
		pm.Unlock()

		time.Sleep(timeout)

		pm.Lock()
		log.Printf("Deleted temporary mapping from %s to %s\n", requestIP, responseIP)
		delete(pm.m, requestIP)
		pm.Unlock()
	}()
}

func (pm *pinMap) PinMapGet(requestIP string) (val string, exists bool) {
	pm.Lock()
	val,exists = pm.m[requestIP]
	pm.Unlock()
	return
}

func dnsHandleFunc(w dns.ResponseWriter, r *dns.Msg) {
	//fmt.Printf("Request: %+v\n",r)
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true


	qt := r.Question[0].Qtype
	qn := r.Question[0].Name
	//fmt.Printf("QType: %d; Name: %s\n", qt, qn)

	switch qt {
	case dns.TypeA:
		resolveIP := "127.0.0.1"
		if sm := reIP.FindStringSubmatch(qn); len(sm) > 1 {
			//extract ip from dns name
			resolveIP = strings.Replace(sm[1], "-", ".", -1)

			//check if there's a stored response
			if rspIP, exists := pm.PinMapGet(resolveIP); exists {
				resolveIP = rspIP
			}
		}
		if sm := reIPpin.FindStringSubmatch(qn); len(sm) == 4 {
			reqIPName := sm[1]
			reqIP := strings.Replace(reqIPName,"-",".", -1)
			rspIPName := sm[2]
			rspIP := strings.Replace(rspIPName, "-", ".", -1)
			timeout,err := time.ParseDuration(sm[3] + "s")
			if err != nil { timeout = 5*time.Second } //failover to 5 seconds pinning
			log.Printf("Lookups (type A) for %s.%s will be answered with %s for %v and with %s afterwards\n", reqIPName, domain, rspIP, timeout, reqIP)
			//Answer with pinned IP
			resolveIP = rspIP
			//add to pinmap
			pm.PinMapAdd(reqIP,rspIP,timeout)
		}
		log.Printf("RESOLVING '%s' to IP %v\n", qn, resolveIP)

		//create resource record
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name: qn,
				Rrtype: dns.TypeA,
				Class: dns.ClassINET,
				Ttl: 0,
			},
			A: net.ParseIP(resolveIP),
		}
		m.Answer = append(m.Answer, rr)
	}
	w.WriteMsg(m)

}




func main() {
	flag.Parse()
	infoText := `******************************************************************************
*              "Let Me In" rebind DNS server by MaMe82                       *
******************************************************************************
* Usage:   lmidns -domain my-rebind-domain.com                               *
*                                                                            *
* From DNS client (example with nslookup):                                   *
* 1) Create request, resolving to arbitrary IPv4 address:                    *
*    Request:   nslookup 192-168-2-1.my-rebind-domain.com                    *
*    Response:  192.168.2.1                                                  *
*                                                                            *
* 2) Temporary pin IP 3.2.4.5 to the same hostname for 10 seconds:           *
*    Request 1:  nslookup 192-168-2-1-to-3-2-4-5-for-10.my-rebind-domain.com *
*    Response 1: 3.2.4.5 (could be ignored)                                  *
*                                                                            *
*    Request 2:  nslookup 192-168-2-1.my-rebind-domain.com                   *
*    Response 2: 3.2.4.5 (answers with pinned IP for 10 seconds)             *
*        ... wait 10 seconds ...                                             *
*    Request 3:  nslookup 192-168-2-1.my-rebind-domain.com                   *
*    Response 3: 192.168.2.1 (answers with IP based on resolved name, again) *
******************************************************************************`

	flag.Usage = func() { fmt.Fprintln(os.Stderr, infoText) }
	flag.Usage()

	fmt.Printf("Serving domain: '%s'\n", *domain)

	//domain = "lmi.mame82.de"
	dns.HandleFunc(*domain, dnsHandleFunc)

	go func() {
		Net := "udp"
		Addr := ":" + strconv.Itoa(*port)
		server := &dns.Server{
			Net: Net,
			Addr: Addr,
		}
		fmt.Printf("Listening on address: '%s'(%s)\n", Addr, Net)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Failed to setup the '%s' server on '%s': %v\n", Net, Addr, err.Error())
		}
	}()

	go func() {
		Net := "tcp"
		Addr := ":" + strconv.Itoa(*port)
		server := &dns.Server{
			Net: "tcp",
			Addr: Addr,
		}
		fmt.Printf("Listening on address: '%s'(%s)\n", Addr, Net)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Failed to setup the '%s' server on '%s': %v\n", Net, Addr, err.Error())
		}
	}()

	//use a channel to wait for SIGTERM or SIGINT
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Fatalf("Signal (%v) received, closing \"Let Me In\" rebind DNS server\n", s)

}