package crypto

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"filippo.io/age"
	"github.com/zeebo/blake3"
)

type memWriter struct {
	buf []byte
}

func (m *memWriter) Write(p []byte) (int, error) {
	m.buf = append(m.buf, p...)
	return len(p), nil
}

func (m *memWriter) Bytes() []byte {
	return m.buf
}

type Blake3NS struct{}

func (Blake3NS) Hash(data []byte) (string, string) {
	hash := blake3.Sum256(data)
	full := hex.EncodeToString(hash[:])
	short := full[:16]
	return full, short
}

func (Blake3NS) HashFile(path string) (string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	h := blake3.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return "", "", err
	}

	sum := h.Sum(nil)
	full := hex.EncodeToString(sum)
	short := full[:16]
	return full, short, nil
}

var Blake3 = Blake3NS{}

type SHA256NS struct{}

func (SHA256NS) Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

var SHA256 = SHA256NS{}

type Ed25519NS struct{}

func (Ed25519NS) Generate() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(nil)
}

func (Ed25519NS) Sign(data []byte, priv ed25519.PrivateKey) []byte {
	return ed25519.Sign(priv, data)
}

func (Ed25519NS) Verify(data, sig []byte, pub ed25519.PublicKey) bool {
	return ed25519.Verify(pub, data, sig)
}

var Ed25519 = Ed25519NS{}

type RSA256NS struct{}

func (RSA256NS) Generate(bits int) (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, bits)
}

func (RSA256NS) Sign(data []byte, priv *rsa.PrivateKey) ([]byte, error) {
	h := sha256.Sum256(data)
	return rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, h[:])
}

func (RSA256NS) Verify(data, sig []byte, pub *rsa.PublicKey) error {
	h := sha256.Sum256(data)
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig)
}

var RSA256 = RSA256NS{}

type AGENS struct{}

func (AGENS) Encrypt(recipientPub string, plaintext []byte) ([]byte, error) {
	r, err := age.ParseX25519Recipient(recipientPub)
	if err != nil {
		return nil, err
	}
	out := &memWriter{}
	encryptor, err := age.Encrypt(out, r)
	if err != nil {
		return nil, err
	}
	_, err = encryptor.Write(plaintext)
	if err != nil {
		return nil, err
	}
	err = encryptor.Close()
	return out.Bytes(), err
}

func (AGENS) Decrypt(identityKey string, ciphertext []byte) ([]byte, error) {
	f, err := os.Open(identityKey)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	identities, err := age.ParseIdentities(f)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(ciphertext)
	decryptor, err := age.Decrypt(r, identities...)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(decryptor)
}

var AGE = AGENS{}
