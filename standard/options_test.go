package standard

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name   string
		config config
	}{
		{name: "empty address", config: config{gracefulTimeout: time.Second}},
		{name: "invalid timeout", config: config{address: ":8080"}},
		{name: "missing private key", config: config{address: ":8080", gracefulTimeout: time.Second, certFile: "server.crt"}},
		{name: "multiple certificate sources", config: config{
			address:         ":8080",
			gracefulTimeout: time.Second,
			certFile:        "server.crt",
			keyFile:         "server.key",
			certificate:     &tls.Certificate{},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.config.validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestWithCertificatePEMRejectsInvalidInput(t *testing.T) {
	cfg := defaultConfig()
	if err := WithCertificatePEM([]byte("invalid"), []byte("invalid"))(&cfg); err == nil {
		t.Fatal("expected invalid pem error")
	}
}

func TestClientConfigValidate(t *testing.T) {
	tests := []clientConfig{
		{tlsConfig: &tls.Config{}, rootCertFile: "root.crt"},
		{rootCertFile: "root.crt", rootCertPEM: []byte("certificate")},
	}
	for _, cfg := range tests {
		if err := cfg.validate(); err == nil {
			t.Fatal("expected client validation error")
		}
	}
}
