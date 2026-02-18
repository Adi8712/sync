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

// Messages

type IndexMessage struct {
	Type     string             `json:"type"`
	DeviceID string             `json:"device_id"`
	Files    []indexer.FileMeta `json:"files"`
}

type FileHeaderMessage struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

type ConsensusVoteMessage struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	Name string `json:"name"`
}

type FileRequestMessage struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	Path string `json:"path"`
}

type DoneMessage struct {
	Type string `json:"type"`
}

// TLS Utilities

func GenerateSelfSignedCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Sync Service"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 365),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})

	return tls.X509KeyPair(certPEM, privPEM)
}

func GetTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
}
