// Copyright (c) 2014 The HADES Authors. All rights reserved.
// Use of this source code is governed by The MIT License (MIT) that can be
// found in the LICENSE file.

package msg

import (
	"net"
	"path"
	"strings"

	"github.com/miekg/dns"
)

// PathPrefix is the prefix used to store HADES data in the backend.

var PathPrefix string = "hades"

// This *is* the rdata from a SRV record, but with a twist.
// Host (Target in SRV) must be a domain name, but if it looks like an IP
// address (4/6), we will treat it like an IP address.
type Service struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Priority int    `json:"priority,omitempty"`
	Weight   int    `json:"weight,omitempty"`
	Text     string `json:"text,omitempty"`
	Mail     bool   `json:"mail,omitempty"` // Be an MX record. Priority becomes Preference.
	Ttl      uint32 `json:"ttl,omitempty"`
	TargetStrip int `json:"targetstrip,omitempty"`

	Group string `json:"group,omitempty"`

	// Etcd key where we found this service and ignored from json un-/marshalling
	Key string `json:"-"`
      // dns type
	Dnstype     string  `json:"type,omitempty"`
}

// NewSRV returns a new SRV record based on the Service.
func (s *Service) NewSRV(name string, weight uint16) *dns.SRV {
	host := targetStrip(dns.Fqdn(s.Host), s.TargetStrip)

	return &dns.SRV{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: s.Ttl},
		Priority: uint16(s.Priority), Weight: weight, Port: uint16(s.Port), Target: host}
}

// NewMX returns a new MX record based on the Service.
func (s *Service) NewMX(name string) *dns.MX {
	host := targetStrip(dns.Fqdn(s.Host), s.TargetStrip)

	return &dns.MX{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: s.Ttl},
		Preference: uint16(s.Priority), Mx: host}
}

// NewA returns a new A record based on the Service.
func (s *Service) NewA(name string, ip net.IP) *dns.A {
	return &dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: s.Ttl}, A: ip}
}

// NewAAAA returns a new AAAA record based on the Service.
func (s *Service) NewAAAA(name string, ip net.IP) *dns.AAAA {
	return &dns.AAAA{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: s.Ttl}, AAAA: ip}
}

// NewCNAME returns a new CNAME record based on the Service.
func (s *Service) NewCNAME(name string, target string) *dns.CNAME {
	return &dns.CNAME{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: s.Ttl}, Target: target}
}

// NewNS returns a new NS record based on the Service.
func (s *Service) NewNS(name string, target string) *dns.NS {
	return &dns.NS{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: s.Ttl}, Ns: target}
}

// NewTXT returns a new TXT record based on the Service.
func (s *Service) NewTXT(name string) *dns.TXT {
	return &dns.TXT{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: s.Ttl}, Txt: split255(s.Text)}
}

// NewPTR returns a new PTR record based on the Service.
func (s *Service) NewPTR(name string, ttl uint32) *dns.PTR {
	return &dns.PTR{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttl}, Ptr: dns.Fqdn(s.Host)}
}

func PathWithWildcard(s string) (string, bool) {
	l := dns.SplitDomainName(s)
	for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
		l[i], l[j] = l[j], l[i]
	}
	for i, k := range l {
		if k == "*" || k == "any" {
			return path.Join(append([]string{"/" + PathPrefix + "/"}, l[:i]...)...), true
		}
	}
	return path.Join(append([]string{"/" + PathPrefix + "/"}, l...)...), false
}

// Path converts a domainname to an etcd path. If s looks like service.staging.hades.local.,
// the resulting key will be /hades/local/hades/staging/service .
func Path(s string) string {
	l := dns.SplitDomainName(s)
	for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
		l[i], l[j] = l[j], l[i]
	}
	return path.Join(append([]string{"/" + PathPrefix + "/"}, l...)...)
}

// Domain is the opposite of Path.
func Domain(s string) string {
	l := strings.Split(s, "/")
	// start with 1, to strip /hades
	for i, j := 1, len(l)-1; i < j; i, j = i+1, j-1 {
		l[i], l[j] = l[j], l[i]
	}
	return dns.Fqdn(strings.Join(l[1:len(l)-1], "."))
}

func Group(sx []Service) []Service {
	if len(sx) == 0 {
		return sx
	}

	group := sx[0].Group
	slashes := strings.Count(sx[0].Key, "/")
	length := make([]int, len(sx))
	for i, s := range sx {
		x := strings.Count(s.Key, "/")
		length[i] = x
		if x < slashes {
			if s.Group == "" {
				break
			}
			slashes = x
			group = s.Group
		}
	}
	if group == "" {
		return sx
	}
	ret := []Service{}

	for i, s := range sx {
		if s.Group == "" {
			ret = append(ret, s)
			continue
		}
		// Disagreement on the same level
		if length[i] == slashes && s.Group != group {
			return sx
		}

		if s.Group == group {
			ret = append(ret, s)
		}
	}
	return ret
}

// Split255 splits a string into 255 byte chunks.
func split255(s string) []string {
	if len(s) < 255 {
		return []string{s}
	}
	sx := []string{}
	p, i := 0, 255
	for {
		if i <= len(s) {
			sx = append(sx, s[p:i])
		} else {
			sx = append(sx, s[p:])
			break

		}
		p, i = p+255, i+255
	}

	return sx
}

// targetStrip strips "targetstrip" labels from the left side of the fully qualified name.
func targetStrip(name string, targetStrip int) string {
	if targetStrip == 0 {
		return name
	}

	offset, end := 0, false
	for i := 0; i < targetStrip; i++ {
		offset, end = dns.NextLabel(name, offset)
	}
	if end {
		// We overshot the name, use the orignal one.
		offset = 0
	}
	name = name[offset:]
	return name
}