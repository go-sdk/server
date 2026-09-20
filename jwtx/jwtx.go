// Package jwtx 提供与传输无关的 JWT 签发和解析，
// 供 server 鉴权中间件和应用侧共用同一套语义。
package jwtx

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"

	"github.com/go-sdk/core/errx"
	"github.com/golang-jwt/jwt/v5"
)

// Signer 以指定算法和密钥签发 token。
type Signer struct {
	Method jwt.SigningMethod
	Key    any
}

// Sign 签发指定 Claims 的 token。
func (s Signer) Sign(claims jwt.Claims) (string, error) {
	if s.Method == nil {
		return "", errx.New("signing method is required")
	}
	if s.Key == nil {
		return "", errx.New("signing key is required")
	}
	return jwt.NewWithClaims(s.Method, claims).SignedString(s.Key)
}

// Parser 按算法白名单和密钥函数解析并校验 token。
type Parser struct {
	KeyFunc      jwt.Keyfunc
	ValidMethods []string
}

// Parse 解析 token 并把结果解码到 claims；签名、算法和有效期校验失败均返回错误。
func (p Parser) Parse(token string, claims jwt.Claims) error {
	if p.KeyFunc == nil {
		return errx.New("key func is required")
	}
	parsed, err := jwt.ParseWithClaims(token, claims, p.KeyFunc, jwt.WithValidMethods(p.ValidMethods))
	if err != nil {
		return errx.Wrap(err, "parse jwt")
	}
	if !parsed.Valid {
		return errx.New("invalid jwt")
	}
	return nil
}

// Codec 同时提供签发和解析能力，对应同一算法和密钥。
type Codec struct {
	Signer
	Parser
}

// HS256 返回以对称密钥签发和解析 HS256 token 的 Codec。
func HS256(secret string) Codec {
	return Codec{
		Method:       jwt.SigningMethodHS256,
		Key:          []byte(secret),
		KeyFunc:      func(*jwt.Token) (any, error) { return []byte(secret), nil },
		ValidMethods: []string{jwt.SigningMethodHS256.Alg()},
	}
}

// Ed25519 从 PEM 编码的 PKCS#8 私钥构造签发和解析 EdDSA token 的 Codec，
// 解析侧使用私钥派生的公钥。
func Ed25519(privateKeyPEM string) (Codec, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return Codec{}, errx.New("invalid ed25519 private key pem")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return Codec{}, errx.Wrap(err, "parse ed25519 private key")
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return Codec{}, errx.New("ed25519 private key pem required")
	}
	return Codec{
		Method:       jwt.SigningMethodEdDSA,
		Key:          privateKey,
		KeyFunc:      func(*jwt.Token) (any, error) { return privateKey.Public(), nil },
		ValidMethods: []string{jwt.SigningMethodEdDSA.Alg()},
	}, nil
}
