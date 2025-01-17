package wallet

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

//go:embed bundle.zip
var bundle embed.FS

const bundleZip = "bundle.zip"

type Config struct {
	Address               string `json:"flowAccountAddress"`
	PrivateKey            string `json:"flowAccountPrivateKey"`
	PublicKey             string `json:"flowAccountPublicKey"`
	AccountKeyID          string `json:"flowAccountKeyId"`
	AccessNode            string `json:"flowAccessNode"`
	BaseURL               string `json:"baseUrl"`
	ContractFungibleToken string `json:"contractFungibleToken"`
	ContractFlowToken     string `json:"contractFlowToken"`
	ContractFUSD          string `json:"contractFUSD"`
	ContractFCLCrypto     string `json:"contractFCLCrypto"`
}

type server struct {
	http   *http.Server
	config *Config
	logger *logrus.Logger
}

// NewHTTPServer returns a new wallet server listening on provided port number.
func NewHTTPServer(port uint, config *Config, logger *logrus.Logger) (*server, error) {
	mux := http.NewServeMux()
	srv := &server{
		http: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		config: config,
		logger: logger,
	}

	mux.HandleFunc("/api/", configHandler(srv))
	mux.HandleFunc("/", devWalletHandler())

	return srv, nil
}

// configHandler handles config endpoints
func configHandler(server *server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(server.config)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// devWalletHandler handles endpoints to exported static html files
func devWalletHandler() func(writer http.ResponseWriter, request *http.Request) {
	zipContent, _ := bundle.ReadFile(bundleZip)
	zipFS, _ := zip.NewReader(bytes.NewReader(zipContent), int64(len(zipContent)))
	rootFS := http.FS(zipFS)

	return func(writer http.ResponseWriter, request *http.Request) {
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path != "" { // api requests don't include .html so that needs to be added
			if _, err := zipFS.Open(path); err != nil {
				path = fmt.Sprintf("%s.html", path)
			}
		}

		request.URL.Path = path
		http.FileServer(rootFS).ServeHTTP(writer, request)
	}
}

func (s *server) Start() error {
	s.logger.WithField("port", "8701").Info("🌱  Starting Dev Wallet Server on port 8701")
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (s *server) Stop() {
	s.http.Shutdown(context.Background())
}
