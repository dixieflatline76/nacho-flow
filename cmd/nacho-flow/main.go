package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/config"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/server"
	"github.com/dixieflatline76/nacho-flow/pkg/store"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
	"github.com/kardianos/service"
)

var version = "0.0.0"

func init() {
	if version != "" {
		contract.Version = version
	}
}

var (
	configPathFlag = flag.String("config", "", "Path to config.yaml file")
	portFlag       = flag.Int("port", 0, "Port to listen on (overrides config.yaml)")
	logLevelFlag   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	versionFlag    = flag.Bool("version", false, "Print version and exit")
	vFlag          = flag.Bool("v", false, "Print version and exit")
)

type program struct {
	server     *http.Server
	tracker    *telemetry.StatsTracker
	store      *store.DiskStore
	trafficLog *telemetry.TrafficLogger
	logCloser  io.Closer
	slog       *slog.Logger
	cancelBg   context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	go p.run(s)
	return nil
}

func parseLogLevel(lvl string) slog.Level {
	switch lvl {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (p *program) run(s service.Service) {
	// Initialize Smart Logger based on Interactive vs Daemon mode
	svcLogger, err := s.Logger(nil)
	if err != nil {
		log.Printf("Service Logger warning: %v", err)
	}

	appLogger, logCloser := telemetry.InitLogger(service.Interactive(), "logs", parseLogLevel(*logLevelFlag), svcLogger)
	p.logCloser = logCloser
	p.slog = appLogger

	cfg, err := config.LoadConfig(*configPathFlag)
	if err != nil {
		appLogger.Error("Config load error", slog.Any("error", err))
		os.Exit(1)
	}

	if *portFlag != 0 {
		cfg.Port = *portFlag
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		appLogger.Error("Evaluator compile error", slog.Any("error", err))
		os.Exit(1)
	}

	// 1. Initialize Provider Registry from config
	reg := provider.NewRegistryFromConfig(cfg)

	// 2. Setup Pricing Oracle with OpenRouter Plugin if configured
	oracle := telemetry.NewPricingOracle()
	if orProvider, ok := cfg.Providers["openrouter"]; ok {
		oracle.RegisterProvider(telemetry.NewOpenRouterPricingProvider(orProvider.APIKey))
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	p.cancelBg = bgCancel
	oracle.StartBackgroundSync(bgCtx, 24*time.Hour)

	// 3. Setup Persistent Disk Store & Async Stats Tracker
	diskStore, err := store.NewDiskStore("")
	if err != nil {
		appLogger.Warn("Failed to initialize stats disk store, running with in-memory stats only", slog.Any("error", err))
	}
	p.store = diskStore

	var initialSnapshot telemetry.StatsSnapshot
	if diskStore != nil {
		loaded, loadErr := diskStore.Load()
		if loadErr == nil {
			initialSnapshot = loaded
			appLogger.Info("Loaded cumulative telemetry from disk",
				slog.Int64("total_requests", loaded.TotalRequests),
				slog.Float64("total_usd_saved", loaded.EstimatedCostSavedUSD),
				slog.String("path", diskStore.FilePath()),
			)
		}
	}

	tracker := telemetry.NewStatsTrackerWithInitialSnapshot(5000, initialSnapshot)
	p.tracker = tracker

	// 4. Attach Streaming TrafficLogger sink for Auto-Tuner
	trafficLogger, err := telemetry.NewTrafficLogger("", 5000)
	if err == nil {
		p.trafficLog = trafficLogger
		tracker.AddSink(trafficLogger)
	} else {
		appLogger.Warn("Failed to initialize traffic logger", slog.Any("error", err))
	}

	// Periodic stats sync to disk (every 1 minute)
	if diskStore != nil {
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-bgCtx.Done():
					return
				case <-ticker.C:
					_ = diskStore.Save(tracker.GetStats())
				}
			}
		}()
	}

	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srvHandler := server.NewServerWithTelemetryAndRegistry(cfg, evaluator, classifier, sanitizer, oracle, tracker, reg, appLogger)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	p.server = &http.Server{
		Addr:              addr,
		Handler:           srvHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	appLogger.Info("🌮 Nacho Flow starting",
		slog.String("address", fmt.Sprintf("http://%s", addr)),
		slog.Int("providers_count", len(reg.All())),
		slog.String("brand", "spicebox.dev"),
	)
	if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		appLogger.Error("HTTP server error", slog.Any("error", err))
	}
}

func (p *program) Stop(s service.Service) error {
	if p.cancelBg != nil {
		p.cancelBg()
	}
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.server.Shutdown(ctx)
	}
	if p.tracker != nil {
		p.tracker.Flush()
		if p.store != nil {
			_ = p.store.Save(p.tracker.GetStats())
		}
		p.tracker.Close()
	}
	if p.trafficLog != nil {
		_ = p.trafficLog.Close()
	}
	if p.logCloser != nil {
		_ = p.logCloser.Close()
	}
	return nil
}

func handleTuneSubcommand(args []string) {
	tuneFlags := flag.NewFlagSet("tune", flag.ExitOnError)
	configPath := tuneFlags.String("config", "config.yaml", "Path to config.yaml file")
	trafficLogPath := tuneFlags.String("traffic-log", "logs/traffic.jsonl", "Path to traffic log JSONL")
	sampleLimit := tuneFlags.Int("sample", 5000, "Maximum historical prompt turns to analyze")
	apply := tuneFlags.Bool("apply", false, "Apply recommended rule optimizations to config.yaml")

	_ = tuneFlags.Parse(args)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config at %s: %v", *configPath, err)
	}

	records, err := telemetry.ReadRecords(*trafficLogPath, *sampleLimit)
	if err != nil {
		log.Fatalf("Failed to read traffic log at %s: %v", *trafficLogPath, err)
	}

	if len(records) == 0 {
		fmt.Printf("ℹ️  No historical traffic records found in %s.\n", *trafficLogPath)
		fmt.Printf("   Run Nacho Flow under normal traffic to accumulate telemetry before tuning.\n")
		return
	}

	optimizer := tuner.NewCostPenaltyOptimizer()
	result, err := optimizer.Optimize(records, cfg)
	if err != nil {
		log.Fatalf("Optimization analysis failed: %v", err)
	}

	// Generate and print human-readable report
	report := tuner.GenerateAdvisoryReport(result, cfg)
	fmt.Print(report)

	if *apply {
		backupPath, err := tuner.ApplyTuning(*configPath, result)
		if err != nil {
			log.Fatalf("Failed to apply tuning: %v", err)
		}
		fmt.Printf("✅ SUCCESS: Successfully updated %s with optimal rules!\n", *configPath)
		fmt.Printf("   Backup saved at: %s\n", backupPath)
		fmt.Printf("   Restart or reload nacho-flow to activate changes.\n\n")
	}
}

func main() {
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		switch cmd {
		case "version", "-v", "--version":
			fmt.Printf("nacho-flow %s (spicebox.dev)\n", contract.Version)
			return
		case "tune":
			handleTuneSubcommand(os.Args[2:])
			return
		}
	}

	flag.Parse()

	if *versionFlag || *vFlag {
		fmt.Printf("nacho-flow %s (spicebox.dev)\n", contract.Version)
		return
	}

	svcConfig := &service.Config{
		Name:        "nacho-flow",
		DisplayName: "Nacho Flow AI Gateway",
		Description: "Ultra-fast hybrid LLM proxy for local GPUs and cloud APIs (spicebox.dev)",
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
					// #nosec G706 - subCmd is a direct CLI flag for service control
					log.Fatalf("[nacho-flow] Service control error (%s): %v", subCmd, err)
				}
				fmt.Printf("[nacho-flow] Service %s executed successfully.\n", subCmd)
				return
			}
		}
	}

	// Handle graceful shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("[nacho-flow] Received shutdown signal, stopping...")
		_ = s.Stop()
		os.Exit(0)
	}()

	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}
