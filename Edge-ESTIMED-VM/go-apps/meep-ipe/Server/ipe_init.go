package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mec "meep-ipe/MEC"
	oneM2M "meep-ipe/oneM2M"

	"github.com/gorilla/handlers"
)

func StartServer(config map[string]interface{}) {
	basePath := config["basePath"].(string)
	port := string(config["ipePort"].(string))
	https := config["https"].(bool)

	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	basePath = strings.TrimSuffix(basePath, "/")
	config["basePath"] = basePath // Update the config with the normalized basePath
	log.Printf("Base path for server: '%s'", basePath)

	router := NewRouter(config)

	addr := fmt.Sprintf(":%s", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: handlers.CORS()(CleanPath(router)),
	}

	// Run the server in a goroutine so it doesn't block signal handling
	go func() {
		if https {
			certPEM, keyPEM, err := generateSelfSignedCert()
			if err != nil {
				log.Fatalf("Failed to generate self-signed cert: %v", err)
			}
			cert, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				log.Fatalf("Failed to load key pair: %v", err)
			}
			srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}

			log.Printf("Starting HTTPS server on %s%s", addr, basePath)
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server: %v", err)
			}
		} else {
			log.Printf("Starting HTTP server on %s%s", addr, basePath)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server: %v", err)
			}
		}
	}()

	// Initialize oneM2M after the server has started
	oneM2M.Init(config)

	// Initialize MEC after the server has started
	mec.InitMEC(config)

	// Wait for interrupt/kill signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	sig := <-stop
	log.Printf("Received signal: %v. Shutting down server...", sig)

	// Register from MEC and oneM2M before shutting down
	mec.TerminateServiceAvailabilityNotificationHandler()
	log.Println("Terminating service availability notification handler...")
	time.Sleep(250 * time.Millisecond)
	mec.TerminateServiceTerminationNotificationHandler()
	log.Println("Terminating service termination notification handler...")
	time.Sleep(250 * time.Millisecond)
	mec.DeleteIotPltf()
	log.Println("Deleting IoT platform...")
	time.Sleep(250 * time.Millisecond)
	err := oneM2M.DeleteNodeSubscription()
	if err != nil {
		log.Printf("Error occurred while deleting node subscription: %v", err)
	}
	log.Println("Deleting node subscription...")
	time.Sleep(250 * time.Millisecond)
	err = oneM2M.DeregisterAE()
	if err != nil {
		log.Printf("Error occurred while deregistering AE: %v", err)
	}
	log.Println("Deregistering AE...")
	time.Sleep(250 * time.Millisecond)
	err = oneM2M.DeleteNode(oneM2M.NodeName())
	if err != nil {
		log.Printf("Error occurred while deleting Node: %v", err)
	}
	log.Println("Deleting Node...")
	time.Sleep(250 * time.Millisecond)
	mec.DeregisterMecService()
	log.Println("Deregistering MEC service...")
	time.Sleep(250 * time.Millisecond)
	mec.DeregisterApplicationInstance()
	log.Println("Deregistering application instance...")
	time.Sleep(250 * time.Millisecond)
	mec.ConfirmTermination()
	log.Println("Confirming termination...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped, port released.")
}

// generateSelfSignedCert creates an in-memory self-signed cert/key pair.
func generateSelfSignedCert() (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Self-Signed Dev Cert"}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("0.0.0.0")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return certPEM, keyPEM, nil
}
