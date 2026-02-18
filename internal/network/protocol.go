package network

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"sync/internal/indexer"
)

type IndexMsg struct {
	Type   string             `json:"t"`
	Device string             `json:"d"`
	Files  []indexer.FileMeta `json:"f"`
}

type FileHeader struct {
	Type string `json:"t"`
	Path string `json:"p"`
	Size int64  `json:"s"`
	Hash string `json:"h"`
}

type VoteMsg struct {
	Type string `json:"t"`
	Hash string `json:"h"`
	Name string `json:"n"`
}

type ReqMsg struct {
	Type string `json:"t"`
	Hash string `json:"h"`
	Path string `json:"p"`
}

func GetCert() (tls.Certificate, error) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Sync"}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, _ := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &k.PublicKey, k)
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	priv := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})

	return tls.X509KeyPair(cert, priv)
}

func TLSConfig(c tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{c}, InsecureSkipVerify: true}
}
