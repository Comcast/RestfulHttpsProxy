/*
Copyright 2019 Comcast Cable Communications Management, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// File is borrowed from goproxy https://github.com/elazarl/goproxy

package proxy

import (
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"runtime"
	"sort"
	"time"
)

func hashSorted(lst []string) []byte {
	c := make([]string, len(lst))
	copy(c, lst)
	sort.Strings(c)
	h := sha1.New()
	for _, s := range c {
		h.Write([]byte(s + ","))
	}
	return h.Sum(nil)
}

func hashSortedBigInt(lst []string) *big.Int {
	rv := new(big.Int)
	rv.SetBytes(hashSorted(lst))
	return rv
}

var goproxySignerVersion = ":goroxy1"

func signHost(ca tls.Certificate, hosts []string) (cert tls.Certificate, err error) {
	var x509ca *x509.Certificate

	// Use the provided ca and not the global GoproxyCa for certificate generation.
	if x509ca, err = x509.ParseCertificate(ca.Certificate[0]); err != nil {
		return
	}
	start := time.Unix(0, 0)
	end, err := time.Parse("2006-01-02", "2049-12-31")
	if err != nil {
		panic(err)
	}
	hash := hashSorted(append(hosts, goproxySignerVersion, ":"+runtime.Version()))
	serial := new(big.Int)
	serial.SetBytes(hash)
	template := x509.Certificate{
		// TODO(elazar): instead of this ugly hack, just encode the certificate and hash the binary form.
		SerialNumber: serial,
		Issuer:       x509ca.Subject,
		Subject: pkix.Name{
			Organization: []string{"RESTful HTTPS Proxy MITM"},
		},
		NotBefore: start,
		NotAfter:  end,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
			template.Subject.CommonName = h
		}
	}
	var csprng CounterEncryptorRand
	if csprng, err = NewCounterEncryptorRandFromKey(ca.PrivateKey, hash); err != nil {
		return
	}
	var certpriv *rsa.PrivateKey
	if certpriv, err = rsa.GenerateKey(&csprng, 2048); err != nil {
		return
	}
	var derBytes []byte
	if derBytes, err = x509.CreateCertificate(&csprng, &template, x509ca, &certpriv.PublicKey, ca.PrivateKey); err != nil {
		return
	}
	return tls.Certificate{
		Certificate: [][]byte{derBytes, ca.Certificate[0]},
		PrivateKey:  certpriv,
	}, nil
}
