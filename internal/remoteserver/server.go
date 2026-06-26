package remoteserver

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"

	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/remoteauth"
	"github.com/mnemon-dev/mnemon/internal/remotesvc"
)

type Server struct {
	svc      remotesvc.Service
	users    *remoteauth.UsersFile
	usersMu  sync.RWMutex
	authPath string
}

func New(svc remotesvc.Service, users *remoteauth.UsersFile, authPath string) *Server {
	return &Server{svc: svc, users: users, authPath: authPath}
}

func (s *Server) authenticate(auth remoteapi.Auth) error {
	s.usersMu.RLock()
	defer s.usersMu.RUnlock()
	return remoteauth.Authenticate(s.users, auth.Principal, auth.Token)
}

func (s *Server) Status(req remoteapi.StatusRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Status()
	resp.JSON = out
	return err
}

func (s *Server) Remember(req remoteapi.RememberRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Remember(req)
	resp.JSON = out
	return err
}

func (s *Server) Recall(req remoteapi.RecallRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Recall(req)
	resp.JSON = out
	return err
}

func (s *Server) Search(req remoteapi.SearchRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Search(req)
	resp.JSON = out
	return err
}

func (s *Server) Link(req remoteapi.LinkRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Link(req)
	resp.JSON = out
	return err
}

func (s *Server) Forget(req remoteapi.ForgetRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Forget(req)
	resp.JSON = out
	return err
}

func (s *Server) Log(req remoteapi.LogRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Log(req)
	resp.JSON = out
	return err
}

func (s *Server) Related(req remoteapi.RelatedRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Related(req)
	resp.JSON = out
	return err
}

func (s *Server) GC(req remoteapi.GCRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.GC(req)
	resp.JSON = out
	return err
}

func (s *Server) Receipt(req remoteapi.ReceiptRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Receipt(req)
	resp.JSON = out
	return err
}

func (s *Server) Embed(req remoteapi.EmbedRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Embed(req)
	resp.JSON = out
	return err
}

func (s *Server) Import(req remoteapi.ImportRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Import(req)
	resp.JSON = out
	return err
}

func (s *Server) Viz(req remoteapi.VizRequest, resp *remoteapi.Response) error {
	if err := s.authenticate(req.Auth); err != nil {
		return err
	}
	out, err := s.svc.Viz(req)
	resp.Text = out
	return err
}

type ServeOptions struct {
	Addr       string
	TLSCert    string
	TLSKey     string
	UsersFile  string
	DataDir    string
	StoreName  string
	EmbedModel string
}

func Serve(opts ServeOptions) error {
	users, err := remoteauth.LoadUsers(opts.UsersFile)
	if err != nil {
		return fmt.Errorf("load users: %w", err)
	}
	cert, err := tls.LoadX509KeyPair(opts.TLSCert, opts.TLSKey)
	if err != nil {
		return fmt.Errorf("load tls keypair: %w", err)
	}
	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName(remoteapi.RPCServiceName, New(remotesvc.Service{
		DataDir:    opts.DataDir,
		StoreName:  opts.StoreName,
		EmbedModel: opts.EmbedModel,
	}, users, opts.UsersFile)); err != nil {
		return err
	}
	ln, err := tls.Listen("tcp", opts.Addr, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	log.Printf("mnemon-server listening on %s", ln.Addr())
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				log.Printf("temporary accept error: %v", err)
				continue
			}
			return err
		}
		go rpcServer.ServeConn(conn)
	}
}
