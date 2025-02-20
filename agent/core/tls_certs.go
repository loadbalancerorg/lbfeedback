// tls_certs.go
// TLS Certificate Management Utilities
//
// Project:     Loadbalancer.org Feedback Agent v5
// Author:      Nicholas Turnbull
//              <nicholas.turnbull@loadbalancer.org>
//
// Copyright (C) 2025 Loadbalancer.org Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"time"
)

func GenerateTLSCertificate() (cert *tls.Certificate, expiresInHours int, err error) {
	// Generate a random serial 128-bit serial number for the cert.
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		err = errors.New("failed to generate serial number: " + err.Error())
		return
	}
	// Build the certificate template with the required configuration.
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Loadbalancer.org Limited"},
		},
		DNSNames:              []string{"localhost"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Duration(int64(expiresInHours) * int64(time.Hour))),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	// Generate an ECDSA private key with the FIPS 186-3 (P256) curve.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		err = errors.New("failed to generate private key: " + err.Error())
		return
	}
	// Create the certificate from the template and private key as a PEM byte array.
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template,
		&key.PublicKey, key,
	)
	if err != nil {
		err = errors.New("failed to generate certificate: " + err.Error())
		return
	}
	// Convert the private key into an X509 PEM byte array.
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		err = errors.New("failed to convert private key to PEM format: " + err.Error())
		return
	}
	// Parse the two PEM formatted blocks into a tls.Certificate object.
	*cert, err = tls.X509KeyPair(certBytes, keyBytes)
	if err != nil {
		err = errors.New("failed to parse public/private key pair: " + err.Error())
		return
	}
	return
}
