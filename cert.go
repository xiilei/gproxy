package gproxy

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"
)

func serialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

// CreateSignCert creates signed cert
func CreateSignCert(
	subject pkix.Name,
	priv interface{},
	ca *tls.Certificate,
	hosts []string) (derBytes []byte, err error) {
	sn, err := serialNumber()
	if err != nil {
		return
	}
	notBefore := time.Now()
	template := &x509.Certificate{
		SerialNumber: sn,
		Subject:      subject,
		NotBefore:    notBefore,
		NotAfter:     notBefore.AddDate(10, 0, 0),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}
	certificate, err := x509.ParseCertificate(ca.Certificate[0])
	if err != nil {
		return
	}
	derBytes, err = x509.CreateCertificate(
		rand.Reader,
		template,
		certificate,
		publicKey(priv),
		ca.PrivateKey)
	if err != nil {
		return
	}
	return
}

// CreateRootCert creates root cert
func CreateRootCert(subject pkix.Name, priv interface{}) (derBytes []byte, err error) {
	sn, err := serialNumber()
	if err != nil {
		return
	}
	notBefore := time.Now()
	template := &x509.Certificate{
		SerialNumber:          sn,
		Subject:               subject,
		NotBefore:             notBefore,
		NotAfter:              notBefore.AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	derBytes, err = x509.CreateCertificate(rand.Reader, template, template, publicKey(priv), priv)
	return
}
