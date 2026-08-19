package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/config"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/server"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/kardianos/service"
)

var (
	configPathFlag = flag.String("config", "", "Path to config.yaml file")
	portFlag       = flag.Int("port", 0, "Port to listen on (overrides config.yaml)")
)

type program struct {
	server *http.Server
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) run() {
	cfg, err := config.LoadConfig(*configPathFlag)
	if err != nil {
		log.Fatalf("[nacho-flow] Config load error: %v", err)
	}

	if *portFlag != 0 {
		cfg.Port = *portFlag
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		log.Fatalf("[nacho-flow] Evaluator compile error: %v", err)
	}

	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srvHandler := server.NewServer(cfg, evaluator, classifier, sanitizer)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	p.server = &http.Server{
		Addr:    addr,
		Handler: srvHandler,
	}

	log.Printf("[nacho-flow] 🌮 Nacho Flow starting on http://%s (spicerack.dev)", addr)
	if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[nacho-flow] HTTP server error: %v", err)
	}
}

func (p *program) Stop(s service.Service) error {
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return p.server.Shutdown(ctx)
	}
	return nil
}

func main() {
	flag.Parse()

	svcConfig := &service.Config{
		Name:        "nacho-flow",
		DisplayName: "Nacho Flow AI Gateway",
		Description: "Ultra-fast hybrid LLM proxy for local GPUs and cloud APIs (spicerack.dev)",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("[nacho-flow] Service initialization error: %v", err)
	}

	// Handle service commands (install, uninstall, start, stop)
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		switch cmd {
		case "service", "svc":
			if len(os.Args) > 2 {
				subCmd := os.Args[2]
				err := service.Control(s, subCmd)
				if err != nil {
					log.Fatalf("[nacho-flow] Service control error (%s): %v", subCmd, err)
				}
				fmt.Printf("[nacho-flow] Service %s executed successfully.\n", subCmd)
				return
			}
		}
	}

	// Run in foreground or as service worker
	logger, err := s.Logger(nil)
	if err != nil {
		log.Printf("Logger warning: %v", err)
	}

	// Handle graceful shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("[nacho-flow] Received shutdown signal, stopping...")
		s.Stop()
		os.Exit(0)
	}()

	err = s.Run()
	if err != nil {
		if logger != nil {
			logger.Error(err)
		} else {
			log.Fatal(err)
		}
	}
}
