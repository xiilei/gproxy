package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"os"

	gp "github.com/xiilei/gproxy"
)

var errorExists = errors.New("name file exists in current dir")

func fileExists(file string) bool {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		logger.Printf("access file faild %s %s \n", file, err)
	} else {
		logger.Printf("file exists %s", file)
	}
	return true
}

func genCA(name, hostname string) (ca tls.Certificate, err error) {
	cacert := name + "-ca.cert"
	cakey := name + "-ca.key"
	if fileExists(cacert) || fileExists(cakey) {
		err = errorExists
		return
	}
	logger.Println("generate self-sign root ca")
	pri, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return
	}
	derBytes, err := gp.CreateRootCert(pkix.Name{
		Organization:  []string{hostname + " CA"},
		Country:       []string{"CN"},
		Province:      []string{"ShangHai"},
		Locality:      []string{"ShangHai"},
		StreetAddress: []string{"Central City"},
	}, pri)
	if err != nil {
		return
	}
	err = writeCertfiles(cacert, cakey, derBytes, pri)
	if err != nil {
		return
	}
	ca = tls.Certificate{
		Certificate: [][]byte{
			derBytes,
		},
		PrivateKey: pri,
	}
	return
}

func generateCert(cacert, cakey, name string, hosts []string) error {
	certfile := name + ".cert"
	keyfile := name + ".key"
	if fileExists(certfile) || fileExists(keyfile) {
		return errorExists
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	var ca tls.Certificate
	// 如果没有ca证书,就创建一个
	if cacert == "" || cakey == "" {
		ca, err = genCA(name, hostname)
	} else {
		ca, err = tls.LoadX509KeyPair(cacert, cakey)
	}
	pri, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	derBytes, err := gp.CreateSignCert(pkix.Name{
		Organization:  []string{hostname},
		Country:       []string{"CN"},
		Province:      []string{"ShangHai"},
		Locality:      []string{"ShangHai"},
		StreetAddress: []string{"Central City"},
	}, pri, &ca, hosts)
	err = writeCertfiles(certfile, keyfile, derBytes, pri)
	if err != nil {
		return err
	}
	logger.Println("generate cert successful!")
	return nil
}

func writeCertfiles(cert, key string, derBytes []byte, pri *rsa.PrivateKey) (err error) {
	certOut, err := os.Create(cert)
	if err != nil {
		return
	}
	if err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return
	}
	if err = certOut.Close(); err != nil {
		return
	}
	keyOut, err := os.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	if err = pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(pri)}); err != nil {
		return
	}
	if err = keyOut.Close(); err != nil {
		return
	}
	return
}
