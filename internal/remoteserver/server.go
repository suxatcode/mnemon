package remoteserver

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/remoteauth"
	"github.com/mnemon-dev/mnemon/internal/store"
)

type Server struct {
	svc      *memorysvc.Service
	verifier remoteauth.Verifier
	db       *store.DB
}

func New(svc *memorysvc.Service, verifier remoteauth.Verifier, db *store.DB) *Server {
	return &Server{svc: svc, verifier: verifier, db: db}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)
	mux.HandleFunc("POST /v1/status", s.auth(s.handleStatus))
	mux.HandleFunc("POST /v1/remember", s.auth(s.handleRemember))
	mux.HandleFunc("POST /v1/recall", s.auth(s.handleRecall))
	mux.HandleFunc("POST /v1/search", s.auth(s.handleSearch))
	mux.HandleFunc("POST /v1/link", s.auth(s.handleLink))
	mux.HandleFunc("POST /v1/forget", s.auth(s.handleForget))
	mux.HandleFunc("POST /v1/log", s.auth(s.handleLog))
	mux.HandleFunc("POST /v1/related", s.auth(s.handleRelated))
	mux.HandleFunc("POST /v1/gc", s.auth(s.handleGC))
	mux.HandleFunc("POST /v1/receipt", s.auth(s.handleReceipt))
	mux.HandleFunc("POST /v1/embed", s.auth(s.handleEmbed))
	mux.HandleFunc("POST /v1/import", s.auth(s.handleImport))
	mux.HandleFunc("POST /v1/viz", s.auth(s.handleViz))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.db == nil || s.db.Ping() != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

type authedHandler func(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor)

func (s *Server) auth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		ident, err := s.verifier.Verify(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err.Error())
			return
		}
		next(w, r, memorysvc.Actor{
			Principal: ident.Principal,
			Role:      ident.Role,
			Agent:     r.Header.Get("X-Mnemon-Agent"),
		})
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	s.reply(w)(s.svc.Status(actor))
}

func (s *Server) handleRemember(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.RememberRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Remember(actor, memorysvc.RememberInput{
		Content: req.Content, Category: req.Category, Importance: req.Importance,
		Tags: req.Tags, Source: req.Source, Entities: req.Entities,
		EntityMode: req.EntityMode, NoDiff: req.NoDiff,
	}))
}

func (s *Server) handleRecall(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.RecallRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Recall(actor, memorysvc.RecallInput{
		Query: req.Query, Category: req.Category, Limit: req.Limit, Source: req.Source,
		Basic: req.Basic, Intent: req.Intent, Verbose: req.Verbose,
	}))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.SearchRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Search(actor, memorysvc.SearchInput{Query: req.Query, Limit: req.Limit}))
}

func (s *Server) handleLink(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.LinkRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Link(actor, memorysvc.LinkInput{
		SourceID: req.SourceID, TargetID: req.TargetID, Type: req.Type, Weight: req.Weight, MetaJSON: req.MetaJSON,
	}))
}

func (s *Server) handleForget(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.ForgetRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Forget(actor, memorysvc.ForgetInput{ID: req.ID}))
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.LogRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Log(actor, memorysvc.LogInput{Limit: req.Limit}))
}

func (s *Server) handleRelated(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.RelatedRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Related(actor, memorysvc.RelatedInput{ID: req.ID, EdgeType: req.EdgeType, Depth: req.Depth}))
}

func (s *Server) handleGC(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.GCRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.GC(actor, memorysvc.GCInput{Threshold: req.Threshold, Limit: req.Limit, KeepID: req.KeepID}))
}

func (s *Server) handleReceipt(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.ReceiptRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Receipt(actor, memorysvc.ReceiptInput{Limit: req.Limit}))
}

func (s *Server) handleEmbed(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.EmbedRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Embed(actor, memorysvc.EmbedInput{ID: req.ID, All: req.All, Status: req.Status}))
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.ImportRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Import(actor, memorysvc.ImportInput{Draft: req.Draft, NoDiff: req.NoDiff, DryRun: req.DryRun}))
}

func (s *Server) handleViz(w http.ResponseWriter, r *http.Request, actor memorysvc.Actor) {
	var req remoteapi.VizRequest
	if !decode(w, r, &req) {
		return
	}
	s.reply(w)(s.svc.Viz(actor, memorysvc.VizInput{Format: req.Format}))
}

func (s *Server) reply(w http.ResponseWriter) func(memorysvc.Result, error) {
	return func(res memorysvc.Result, err error) {
		if err != nil {
			code := http.StatusBadRequest
			msg := err.Error()
			if strings.HasPrefix(msg, "forbidden:") {
				code = http.StatusForbidden
			}
			writeErr(w, code, msg)
			return
		}
		env := remoteapi.Envelope{Warnings: res.Warnings, Text: res.Text}
		if len(res.JSON) > 0 {
			var parsed any
			if json.Unmarshal(res.JSON, &parsed) == nil {
				env.Result = parsed
			} else {
				env.Result = json.RawMessage(res.JSON)
			}
		}
		writeJSON(w, http.StatusOK, env)
	}
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body")
		return false
	}
	if len(body) == 0 {
		return true
	}
	if err := json.Unmarshal(body, dest); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, remoteapi.Envelope{Error: msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

type ServeOptions struct {
	Addr        string
	TLSCert     string
	TLSKey      string
	DataDir     string
	StoreName   string
	DatabaseURL string
	EmbedModel  string
	JWTKeyFile  string
	MaxInsights int
}

func Serve(opts ServeOptions) error {
	db, err := store.OpenWithOptions(store.Options{
		DataDir:     storeDir(opts.DataDir, opts.StoreName),
		DatabaseURL: opts.DatabaseURL,
	})
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()
	key, err := remoteauth.LoadKey(opts.JWTKeyFile)
	if err != nil {
		return fmt.Errorf("jwt key: %w", err)
	}
	max := opts.MaxInsights
	if max <= 0 {
		max = store.MaxInsightsFromEnv(store.ServerDefaultMaxInsights)
	}
	svc := memorysvc.New(db, memorysvc.Options{
		EmbedModel:  opts.EmbedModel,
		MaxInsights: max,
		EnforceACL:  true,
		StoreName:   opts.StoreName,
	})
	handler := New(svc, remoteauth.StoreVerifier{DB: db, Key: key}, db).Handler()
	srv := &http.Server{
		Addr:              opts.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ln, err := listen(opts)
	if err != nil {
		return err
	}
	log.Printf("mnemon-server listening on %s", ln.Addr())
	return srv.Serve(ln)
}

func listen(opts ServeOptions) (net.Listener, error) {
	if opts.TLSCert == "" || opts.TLSKey == "" {
		return net.Listen("tcp", opts.Addr)
	}
	cert, err := tls.LoadX509KeyPair(opts.TLSCert, opts.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("load tls keypair: %w", err)
	}
	return tls.Listen("tcp", opts.Addr, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	})
}

func storeDir(dataDir, storeName string) string {
	if storeName == "" {
		storeName = store.DefaultStoreName
	}
	if dataDir == "" {
		dataDir = store.DefaultDataDir()
	}
	return store.StoreDir(dataDir, storeName)
}
