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

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"
)

func createPemFiles() (*tls.Certificate, error) {
	serialNumber := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumber)

	if err != nil {
		return nil, err
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey

	ca := &x509.Certificate{
		SerialNumber: serialNumber,

		Subject: pkix.Name{
			Organization: []string{"Restful proxy"},
			CommonName:   "Restful proxy",
		},

		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(20, 0, 0),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		IsCA: true,
	}

	caPemBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, priv)
	if err != nil {
		return nil, err
	}
	caPemBytes = pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caPemBytes,
		},
	)
	keyPemBytes := x509.MarshalPKCS1PrivateKey(priv)
	keyPemBytes = pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: keyPemBytes,
		},
	)

	// Public key
	caPemFile, err := os.Create("ca.pem")
	//pem.Encode(caPemFile, &pem.Block{Type: "CERTIFICATE", Bytes: caPemBytes})
	_, err = caPemFile.Write(caPemBytes)
	if err != nil {
		return nil, err
	}
	err = caPemFile.Close()
	if err != nil {
		return nil, err
	}

	// Private key
	keyPemFile, err := os.OpenFile("key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	//pem.Encode(keyPemFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyPemBytes})
	_, err = keyPemFile.Write(keyPemBytes)
	if err != nil {
		return nil, err
	}
	err = keyPemFile.Close()
	if err != nil {
		return nil, err
	}

	cert, err := tls.X509KeyPair(caPemBytes, keyPemBytes)

	if err != nil {
		return nil, err
	}

	return &cert, err
}
