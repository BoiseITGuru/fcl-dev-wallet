package wallet

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/bjartek/overflow/overflow"
	"github.com/gorilla/mux"
	"github.com/onflow/flow-cli/pkg/flowkit"
	"github.com/onflow/flow-cli/pkg/flowkit/config"
	"github.com/onflow/flow-cli/pkg/flowkit/util"
	"github.com/onflow/flow-go-sdk"
	"github.com/onflow/flow-go-sdk/crypto"
	"github.com/sirupsen/logrus"
)

//go:embed bundle.zip
var bundle embed.FS

// TODO: flow config should already be decided before dev-wallet starts
//go:embed wallet-app/flow.json
var flowConfig []byte

const bundleZip = "bundle.zip"

type Config struct {
	Address    string `json:"flowAccountAddress"`
	PrivateKey string `json:"flowAccountPrivateKey"`
	PublicKey  string `json:"flowAccountPublicKey"`
	AccessNode string `json:"flowAccessNode"`
	Accounts   struct {
		Service struct {
			Address string `json:"address"`
			Key     string `json:"key"`
		} `json:"emulator-account"`
	}
	Contracts map[string]string `json:"contracts"`
}

type server struct {
	http     *http.Server
	config   *Config
	logger   *logrus.Logger
	overflow *overflow.Overflow
}

type FclAccount struct {
	Type    string    `json:"type"`
	Address string    `json:"address"`
	KeyId   int       `json:"keyId"`
	Label   string    `json:"label"`
	Scopes  *[]string `json:"scopes"`
}

type fclAccounts []FclAccount

// TODO: flow config should already be decided before dev-wallet starts
var tempFlowConfig string

func checkFlowConfig() {
	if _, e := os.Stat("flow.json"); os.IsNotExist(e) {
		tempConfig, err := os.CreateTemp("", "flow-*.json")
		if err != nil {
			log.Fatal(err)
		}

		if _, err := tempConfig.Write(flowConfig); err != nil {
			log.Fatal(err)
		}

		tempConfig.Close()

		tempFlowConfig = tempConfig.Name()
	}
}

// NewHTTPServer returns a new wallet server listening on provided port number.
func NewHTTPServer(port uint, config *Config, logger *logrus.Logger) (*server, error) {
	mux := mux.NewRouter()
	srv := &server{
		http: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		config: config,
		logger: logger,
	}

	api := mux.PathPrefix("/api").Subrouter()
	api.HandleFunc("/", configHandler(srv))
	api.HandleFunc("/accounts", getAllAccountsHandler(srv))
	api.HandleFunc("/accounts/{address}", getAccountHandler(srv))
	api.HandleFunc("/accounts/{address}/update/{name}", updateAccountHandler(srv))
	api.HandleFunc("/accounts/{address}/delete", deleteAccountHandler(srv))
	api.HandleFunc("/accounts/{address}/fund", fundAccountHandler(srv))
	api.HandleFunc("/accounts/{address}/fusd", fusdAccountHandler(srv))
	api.HandleFunc("/accounts/create/{name}", createAccountHandler(srv))

	mux.PathPrefix("/").HandlerFunc(devWalletHandler())

	return srv, nil
}

func createAccountHandler(server *server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		o := server.overflow

		sigAlgo := crypto.StringToSignatureAlgorithm("ECDSA_P256")
		hashAlgo := crypto.StringToHashAlgorithm("SHA3_256")
		seed, err := util.RandomSeed(crypto.MinSeedLength)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		privateKey, err := crypto.GeneratePrivateKey(sigAlgo, seed)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		pubKey := privateKey.PublicKey()

		signerAccount, err := o.State.Accounts().ByName(o.ServiceAccountName())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		account, err := o.Services.Accounts.Create(
			signerAccount,
			[]crypto.PublicKey{pubKey},
			[]int{1000},
			[]crypto.SignatureAlgorithm{sigAlgo},
			[]crypto.HashAlgorithm{hashAlgo},
			[]string{},
		)

		acctKey := config.AccountKey{
			Type:       "hex",
			Index:      0,
			SigAlgo:    sigAlgo,
			HashAlgo:   hashAlgo,
			PrivateKey: privateKey,
		}

		flowkitAcctKey, err := flowkit.NewAccountKey(acctKey)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		flowkitAcct := &flowkit.Account{}
		flowkitAcct.SetAddress(account.Address)
		flowkitAcct.SetName(vars["name"])
		flowkitAcct.SetKey(flowkitAcctKey)

		o.State.Accounts().AddOrUpdate(flowkitAcct)

		fclAccount := FclAccount{
			Type:    "ACCOUNT",
			Address: account.Address.String(),
			KeyId:   0,
			Label:   flowkitAcct.Name(),
			Scopes:  new([]string),
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		errJson := json.NewEncoder(w).Encode(fclAccount)
		if errJson != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

// TODO:
func fusdAccountHandler(server *server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		errJson := json.NewEncoder(w).Encode("OK")
		if errJson != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}

}

// TODO:
func fundAccountHandler(server *server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		errJson := json.NewEncoder(w).Encode("OK")
		if errJson != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func deleteAccountHandler(server *server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		s := server.overflow.State

		address := flow.HexToAddress(vars["address"])
		acct, err := s.Accounts().ByAddress(address)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = server.overflow.State.Accounts().Remove(acct.Name())
		if err == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		errJson := json.NewEncoder(w).Encode("OK")
		if errJson != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func updateAccountHandler(server *server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		address := flow.HexToAddress(vars["address"])
		account, err := server.overflow.State.Accounts().ByAddress(address)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		account.SetName(vars["name"])

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		errJson := json.NewEncoder(w).Encode("OK")
		if errJson != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func getAccountHandler(server *server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		address := flow.HexToAddress(vars["address"])
		account, err := server.overflow.State.Accounts().ByAddress(address)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		errJson := json.NewEncoder(w).Encode(account)
		if errJson != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func getAllAccountsHandler(server *server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fclAccountList := []FclAccount{}

		for _, account := range *server.overflow.State.Accounts() {
			fclAccount := FclAccount{
				Type:    "ACCOUNT",
				Address: account.Address().String(),
				KeyId:   0,
				Label:   account.Name(),
				Scopes:  new([]string),
			}

			fclAccountList = append(fclAccountList, fclAccount)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(fclAccountList)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
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
	// rootFS := http.FS(zipFS)

	return func(writer http.ResponseWriter, request *http.Request) {
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path != "" { // api requests don't include .html so that needs to be added
			if _, err := zipFS.Open(path); err != nil {
				path = fmt.Sprintf("%s.html", path)
			}
		}

		request.URL.Path = path
		http.FileServer(http.Dir("./wallet-app/out")).ServeHTTP(writer, request)
	}
}

func (s *server) Start() error {
	//Overflow start up
	var overflowConfig *overflow.OverflowBuilder

	if tempFlowConfig != "" {
		overflowConfig = overflow.NewOverflowBuilder("emulator", false, 0).Config(tempFlowConfig)
	} else {
		overflowConfig = overflow.NewOverflowBuilder("emulator", false, 0)
	}

	overflowConfig.InitializeAccounts = true
	overflowConfig.DeployContracts = false

	s.overflow = overflowConfig.Start()

	//Dev Wallet UI and API Start Up
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
