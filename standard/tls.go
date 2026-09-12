package standard

import (
	"crypto/tls"
	"crypto/x509"
	"net"

	"github.com/go-sdk/core/errx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func (s *Server) gatewayDialOptions(endpoint string) ([]grpc.DialOption, error) {
	var opts []grpc.DialOption
	if !s.usesTLS() {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		serverName := s.config.gatewayServerName
		if serverName == "" {
			serverName, _, _ = net.SplitHostPort(endpoint)
		}
		if s.config.certFile != "" {
			transportCredentials, err := credentials.NewClientTLSFromFile(s.config.certFile, serverName)
			if err != nil {
				return nil, errx.Wrap(err, "load gateway tls certificate")
			}
			opts = append(opts, grpc.WithTransportCredentials(transportCredentials))
		} else if len(s.config.certificatePEM) != 0 {
			roots, err := x509.SystemCertPool()
			if err != nil {
				roots = x509.NewCertPool()
			}
			if !roots.AppendCertsFromPEM(s.config.certificatePEM) {
				return nil, errx.New("load gateway tls certificate pem")
			}
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    roots,
				ServerName: serverName,
			})))
		} else {
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				MinVersion: tls.VersionTLS12,
				ServerName: serverName,
			})))
		}
	}
	return append(opts, s.config.gatewayDialOptions...), nil
}

func (s *Server) usesTLS() bool {
	return s.config.certFile != "" || s.config.certificate != nil || s.config.tlsConfig != nil
}
