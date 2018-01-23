package listener

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"

	"errors"

	"strconv"

	"github.com/rancher/norman/types/slice"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/acme/autocert"
)

const (
	httpsMode = "https"
	httpMode  = "http"
	acmeMode  = "acme"
)

type Server struct {
	sync.Mutex

	handler             HandlerGetter
	httpPort, httpsPort int

	listeners    []net.Listener
	servers      []*http.Server
	activeConfig *v3.ListenConfig
	activeMode   string

	// dynamic config change on refresh
	activeCert *tls.Certificate
	domains    map[string]bool
	tos        []string
	tosAll     bool
}

func NewServer(handler HandlerGetter, httpPort, httpsPort int) *Server {
	return &Server{
		handler:   handler,
		httpPort:  httpPort,
		httpsPort: httpsPort,
	}
}

func (s *Server) Disable(config *v3.ListenConfig) {
	if s.activeConfig == nil {
		return
	}

	if s.activeConfig.UID == config.UID {
		s.activeConfig = nil
	}
}

func (s *Server) Enable(config *v3.ListenConfig) (bool, error) {
	s.Lock()
	defer s.Unlock()

	if s.activeConfig != nil && s.activeConfig.CreationTimestamp.Before(&config.CreationTimestamp) {
		return false, nil
	}

	s.domains = map[string]bool{}
	for _, d := range config.Domains {
		s.domains[d] = true
	}

	s.tos = config.TOS
	s.tosAll = slice.ContainsString(config.TOS, "auto") || slice.ContainsString(config.TOS, "")

	if config.Key != "" && config.Cert != "" {
		cert, err := tls.X509KeyPair([]byte(config.Cert), []byte(config.Key))
		if err != nil {
			return false, err
		}
		s.activeCert = &cert
	}

	if s.activeConfig == nil || config.Mode != s.activeMode {
		return true, s.reload(config)
	}
	return true, nil
}

func (s *Server) hostPolicy(ctx context.Context, host string) error {
	s.Lock()
	defer s.Unlock()

	if s.domains[host] {
		return nil
	}

	return errors.New("acme/autocert: host not configured")
}

func (s *Server) prompt(tos string) bool {
	s.Lock()
	defer s.Unlock()

	if s.tosAll {
		return true
	}

	return slice.ContainsString(s.tos, tos)
}

func (s *Server) Shutdown() error {
	for _, listener := range s.listeners {
		if err := listener.Close(); err != nil {
			return err
		}
	}
	s.listeners = nil

	for _, server := range s.servers {
		go server.Shutdown(context.Background())
	}
	s.servers = nil

	return nil
}

func (s *Server) reload(config *v3.ListenConfig) error {
	if err := s.Shutdown(); err != nil {
		return err
	}

	switch config.Mode {
	case acmeMode:
		if err := s.serveACME(config); err != nil {
			return err
		}
	case httpMode:
		if err := s.serveHTTP(config); err != nil {
			return err
		}
	case httpsMode:
		if err := s.serveHTTPS(config); err != nil {
			return err
		}
	}

	s.activeMode = config.Mode
	s.activeConfig = config
	return nil
}

func (s *Server) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.Lock()
	defer s.Unlock()

	return s.activeCert, nil
}

func (s *Server) serveHTTPS(config *v3.ListenConfig) error {
	conf := &tls.Config{
		GetCertificate: s.getCertificate,
	}
	addr := fmt.Sprintf(":%d", s.httpsPort)

	listener, err := tls.Listen("tcp", addr, conf)
	if err != nil {
		return err
	}

	s.listeners = append(s.listeners)
	logrus.Info("Listening on ", addr)

	server := &http.Server{
		Handler: s.Handler(),
	}

	s.servers = append(s.servers, server)
	s.startServer(listener, server)

	httpListener, err := s.newListener(s.httpPort)
	if err != nil {
		return err
	}
	s.listeners = append(s.listeners, httpListener)

	httpServer := &http.Server{
		Handler: http.HandlerFunc(httpRedirect),
	}

	s.servers = append(s.servers, httpServer)
	s.startServer(httpListener, httpServer)

	return nil
}

// Approach taken from letsencrypt, except manglePort is specific to us
func httpRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Use HTTPS", http.StatusBadRequest)
		return
	}
	target := "https://" + manglePort(r.Host) + r.URL.RequestURI()
	http.Redirect(w, r, target, http.StatusFound)
}

func manglePort(hostport string) string {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return hostport
	}

	portInt = (portInt / 1000) + 443

	return net.JoinHostPort(host, strconv.Itoa(portInt))
}

func (s *Server) serveHTTP(config *v3.ListenConfig) error {
	listener, err := s.newListener(s.httpPort)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: s.Handler(),
	}
	s.servers = append(s.servers, server)
	s.startServer(listener, server)
	return nil
}

func (s *Server) startServer(listener net.Listener, server *http.Server) {
	go func() {
		if err := server.Serve(listener); err != nil {
			logrus.Errorf("server on %v returned err: %v", listener.Addr(), err)
		}
	}()
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		h := s.handler()
		if h == nil {
			rw.WriteHeader(http.StatusServiceUnavailable)
		} else {
			h.ServeHTTP(rw, req)
		}
	})
}

func (s *Server) newListener(port int) (net.Listener, error) {
	addr := fmt.Sprintf(":%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s.listeners = append(s.listeners, l)
	logrus.Info("Listening on ", addr)

	return l, nil
}

func (s *Server) serveACME(config *v3.ListenConfig) error {
	manager := autocert.Manager{
		Cache:      autocert.DirCache("certs-cache"),
		Prompt:     s.prompt,
		HostPolicy: s.hostPolicy,
	}
	conf := &tls.Config{
		GetCertificate: manager.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
	}

	addr := fmt.Sprintf(":%d", s.httpsPort)
	httpsListener, err := tls.Listen("tcp", addr, conf)
	if err != nil {
		return err
	}
	s.listeners = append(s.listeners, httpsListener)
	logrus.Info("Listening on ", addr)

	httpListener, err := s.newListener(s.httpPort)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Handler: manager.HTTPHandler(nil),
	}
	s.servers = append(s.servers, httpServer)
	go func() {
		if err := httpServer.Serve(httpListener); err != nil {
			logrus.Errorf("http server returned err: %v", err)
		}
	}()

	httpsServer := &http.Server{
		Handler: s.Handler(),
	}
	s.servers = append(s.servers, httpsServer)
	go func() {
		if err := httpsServer.Serve(httpsListener); err != nil {
			logrus.Errorf("https server returned err: %v", err)
		}
	}()

	return nil
}
