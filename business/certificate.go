package business

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xmx/aegis-broker/applet/errcode"
	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func NewCertificate(repo repository.All, log *slog.Logger) *Certificate {
	return &Certificate{
		repo: repo,
		log:  log,
	}
}

type Certificate struct {
	repo  repository.All
	log   *slog.Logger
	mutex sync.Mutex
	mana  atomic.Pointer[certificateManager]
	self  atomic.Pointer[tls.Certificate] // 自签名证书。
}

func (crt *Certificate) GetCertificate(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	serverName := chi.ServerName
	attrs := []any{slog.String("server_name", serverName)}

	mana := crt.mana.Load()
	if mana == nil {
		mana = crt.loadManager()
	}

	if cert, err := mana.GetCertificate(chi); err != nil {
		args := append(attrs, slog.Any("error", err))
		crt.log.Warn("从管理器中匹配证书错误", args...)
	} else if cert != nil {
		return cert, nil
	} else {
		crt.log.Warn("从管理器中未匹配到任何证书", attrs...)
	}

	// 没有配置证书或证书加载错误时，回退到自签名证书，优先保证服务能够访问。
	if self := crt.self.Load(); self != nil {
		return self, nil
	}
	crt.mutex.Lock()
	defer crt.mutex.Unlock()
	if self := crt.self.Load(); self != nil {
		return self, nil
	}

	crt.log.Warn("开始生成自签名证书", attrs...)
	self, err := crt.generate()
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
		crt.log.Warn("生成自签名证书错误", attrs...)
	} else {
		crt.self.Store(self)
	}

	return self, err
}

func (crt *Certificate) Forget() {
	crt.mana.Store(nil)
}

func (crt *Certificate) Parse(publicKey, privateKey string) (*model.Certificate, error) {
	pubKey, priKey := []byte(publicKey), []byte(privateKey)
	cert, err := tls.X509KeyPair(pubKey, priKey)
	if err != nil {
		return nil, errcode.ErrCertificateInvalid
	}

	leaf := cert.Leaf
	sub := leaf.Subject
	ips := make([]string, 0, len(leaf.IPAddresses))
	for _, addr := range leaf.IPAddresses {
		ips = append(ips, addr.String())
	}
	uris := make([]string, 0, len(leaf.URIs))
	for _, uri := range leaf.URIs {
		uris = append(uris, uri.String())
	}

	// 计算指纹
	certSHA256, pubKeySHA256, priKeySHA256 := crt.fingerprintSHA256(cert)
	dat := &model.Certificate{
		CommonName:         sub.CommonName,
		PublicKey:          publicKey,
		PrivateKey:         privateKey,
		CertificateSHA256:  certSHA256,
		PublicKeySHA256:    pubKeySHA256,
		PrivateKeySHA256:   priKeySHA256,
		DNSNames:           leaf.DNSNames,
		IPAddresses:        ips,
		EmailAddresses:     leaf.EmailAddresses,
		URIs:               uris,
		Version:            leaf.Version,
		NotBefore:          leaf.NotBefore,
		NotAfter:           leaf.NotAfter,
		Issuer:             crt.parsePKIX(leaf.Issuer),
		Subject:            crt.parsePKIX(leaf.Subject),
		SignatureAlgorithm: cert.Leaf.SignatureAlgorithm.String(),
	}

	return dat, nil
}

func (*Certificate) parsePKIX(v pkix.Name) model.CertificatePKIXName {
	return model.CertificatePKIXName{
		Country:            v.Country,
		Organization:       v.Organization,
		OrganizationalUnit: v.OrganizationalUnit,
		Locality:           v.Locality,
		Province:           v.Province,
		StreetAddress:      v.StreetAddress,
		PostalCode:         v.PostalCode,
		SerialNumber:       v.SerialNumber,
		CommonName:         v.CommonName,
	}
}

// fingerprintSHA256 计算证书和私钥的 SHA256 指纹。
func (*Certificate) fingerprintSHA256(cert tls.Certificate) (certSHA256, pubKeySHA256, priKeySHA256 string) {
	leaf := cert.Leaf
	sum256 := sha256.Sum256(leaf.Raw)
	certSHA256 = hex.EncodeToString(sum256[:])

	if pki, _ := x509.MarshalPKIXPublicKey(leaf.PublicKey); len(pki) != 0 {
		sum := sha256.Sum256(pki)
		pubKeySHA256 = hex.EncodeToString(sum[:])
	}

	if pkcs8, _ := x509.MarshalPKCS8PrivateKey(cert.PrivateKey); len(pkcs8) != 0 {
		sum := sha256.Sum256(pkcs8)
		priKeySHA256 = hex.EncodeToString(sum[:])
	}

	return
}

func (crt *Certificate) loadManager() *certificateManager {
	crt.mutex.Lock()
	defer crt.mutex.Unlock()
	if mana := crt.mana.Load(); mana != nil {
		return mana
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mana := &certificateManager{certs: make(map[string][]*tls.Certificate)}
	repo := crt.repo.Certificate()
	dats, err := repo.Find(ctx, bson.M{"enabled": true})
	if err != nil {
		mana.err = fmt.Errorf("查询已启用证书错误: %w", err)
		crt.mana.Store(mana)
		return mana
	} else if len(dats) == 0 {
		mana.err = errcode.ErrCertificateUnavailable
		crt.mana.Store(mana)
		return mana
	}

	for _, dat := range dats {
		pubKey, priKey := []byte(dat.PublicKey), []byte(dat.PrivateKey)
		pair, exx := tls.X509KeyPair(pubKey, priKey)
		if exx != nil {
			err = exx
			continue
		}

		leaf := pair.Leaf
		for _, name := range leaf.DNSNames {
			cts := mana.certs[name]
			mana.certs[name] = append(cts, &pair)
		}
		for _, ip := range leaf.IPAddresses {
			name := ip.String()
			cts := mana.certs[name]
			mana.certs[name] = append(cts, &pair)
		}
	}
	if len(mana.certs) == 0 {
		mana.err = err
	}
	crt.mana.Store(mana)

	return mana
}

func (*Certificate) generate() (*tls.Certificate, error) {
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "aegis",
			Organization: []string{"aegis"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"server.aegis.internal"},
		IPAddresses:           []net.IP{{127, 0, 0, 1}},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tlsCert, nil
}

type certificateManager struct {
	err   error
	certs map[string][]*tls.Certificate
}

func (cm *certificateManager) GetCertificate(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if cm.err != nil {
		return nil, cm.err
	}
	crt := cm.match(chi.ServerName)

	return crt, nil
}

func (cm *certificateManager) match(serverName string) *tls.Certificate {
	now := time.Now()
	// https://github.com/golang/go/blob/go1.22.5/src/crypto/tls/common.go#L1141-L1154
	name := strings.ToLower(serverName)
	var last *tls.Certificate
	for _, crt := range cm.certs[name] {
		notBefore, notAfter := crt.Leaf.NotBefore, crt.Leaf.NotAfter
		if notBefore.After(now) && notAfter.Before(now) {
			return crt
		}
		last = crt
	}

	labels := strings.Split(name, ".")
	labels[0] = "*"
	wildcardName := strings.Join(labels, ".")
	for _, crt := range cm.certs[wildcardName] {
		notBefore, notAfter := crt.Leaf.NotBefore, crt.Leaf.NotAfter
		if notBefore.After(now) && notAfter.Before(now) {
			return crt
		}
		last = crt
	}

	return last
}
