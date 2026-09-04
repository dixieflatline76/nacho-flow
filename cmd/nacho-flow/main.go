// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
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
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
	"github.com/kardianos/service"
)

var version = "0.0.0"

func init() {
	if version != "" {
		contract.Version = version
	}
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of nacho-flow:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "\nSubcommands:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  deals         Find drop-in model replacements for your tiers (Heat Seeker)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  heat-seek     Alias for 'deals'\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  tune          Analyze traffic logs and synthesize optimal routing rules\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  service       Manage background OS daemon (install, uninstall, start, stop)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  version       Print version information and exit\n")
		fmt.Fprintf(flag.CommandLine.Output(), "\nFlags:\n")
		flag.PrintDefaults()
	}
}

var (
	configPathFlag         = flag.String("config", "", "Path to config.yaml file")
	portFlag               = flag.Int("port", 0, "Port to listen on (overrides config.yaml)")
	logLevelFlag           = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	versionFlag            = flag.Bool("version", false, "Print version and exit")
	vFlag                  = flag.Bool("v", false, "Print version and exit")
	logFatal               = log.Fatal
	applyTuningFunc        = tuner.ApplyTuning
	serviceNewFunc         = service.New
	serviceInteractiveFunc = service.Interactive
)

type program struct {
	mu                sync.Mutex
	server            *http.Server
	tracker           *telemetry.StatsTracker
	store             *store.DiskStore
	trafficLog        *telemetry.TrafficLogger
	logCloser         io.Closer
	slog              *slog.Logger
	cancelBg          context.CancelFunc
	statsSyncInterval time.Duration
	onExit            chan struct{}
}

func (p *program) Start(s service.Service) error {
	go p.asyncRun(s)
	return nil
}

func (p *program) asyncRun(s service.Service) {
	defer func() {
		p.mu.Lock()
		ch := p.onExit
		p.mu.Unlock()
		if ch != nil {
			close(ch)
		}
	}()
	if err := p.run(s); err != nil {
		p.mu.Lock()
		sl := p.slog
		p.mu.Unlock()
		if sl == nil {
			sl = slog.Default()
		}
		sl.Error("Fatal runtime error", slog.Any("error", err))
	}
}

func isAddressInUse(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			var errno syscall.Errno
			if errors.As(sysErr.Err, &errno) {
				// WSAEADDRINUSE (10048) on Windows, EADDRINUSE on Unix (98 Linux, 48 Darwin)
				return errno == syscall.EADDRINUSE || errno == 10048
			}
		}
	}
	return errors.Is(err, syscall.EADDRINUSE)
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

func (p *program) run(s service.Service) error {
	// Initialize Smart Logger based on Interactive vs Daemon mode
	var svcLogger service.Logger
	if s != nil {
		var err error
		svcLogger, err = s.Logger(nil)
		if err != nil {
			log.Printf("Service Logger warning: %v", err)
		}
	}

	appLogger, logCloser := telemetry.InitLogger(serviceInteractiveFunc(), "logs", parseLogLevel(*logLevelFlag), svcLogger)
	p.mu.Lock()
	p.logCloser = logCloser
	p.slog = appLogger
	p.mu.Unlock()

	cfg, err := config.LoadConfig(*configPathFlag)
	if err != nil {
		if serviceInteractiveFunc() {
			fmt.Printf("\n🌮 Nacho Flow %s (https://spicebox.dev/nacho-flow/)\n\n", contract.Version)
			if strings.Contains(err.Error(), "could not find") {
				fmt.Println("No configuration file found. To get started:")
			} else {
				fmt.Printf("Configuration error: %v\n\n", err)
				fmt.Println("To fix your configuration:")
			}
			fmt.Println("  1. Create a config.yaml or specify one with: nacho-flow -config path/to/config.yaml")
			fmt.Println("  2. Run 'nacho-flow -help' for available options.")
			fmt.Println("  3. View documentation at https://spicebox.dev/nacho-flow/docs.html")
			return nil
		}
		fmt.Fprintf(os.Stderr, "[FATAL:CONFIG_ERROR] %v\n", err)
		appLogger.Error("Config load error", slog.Any("error", err))
		return err
	}

	if *portFlag != 0 {
		cfg.Port = *portFlag
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier, cfg.Providers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL:RULE_ERROR] %v\n", err)
		appLogger.Error("Evaluator compile error", slog.Any("error", err))
		return err
	}

	// 1. Initialize Provider Registry from config
	reg := provider.NewRegistryFromConfig(cfg)

	// 2. Setup Curated Gallery, Classifier, and Pricing Oracle with OpenRouter Plugin
	curationMgr := curation.NewManager("", "")
	modelClassifier := telemetry.NewClassifier(curationMgr)
	oracle := telemetry.NewPricingOracleWithClassifier(modelClassifier)
	for id, p := range cfg.Providers {
		if factory, ok := telemetry.LookupPricingFactory(id); ok {
			prov, syncInterval := factory(id, p, 0)
			oracle.RegisterProvider(prov, syncInterval)
		}
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.cancelBg = bgCancel
	p.mu.Unlock()
	oracle.StartBackgroundSync(bgCtx, 24*time.Hour)

	// Trigger async OTA catalog refresh
	go func() {
		_, _ = curationMgr.SyncOTA(bgCtx)
	}()

	// 3. Setup Persistent Disk Store & Async Stats Tracker
	diskStore, err := store.NewDiskStore("")
	if err != nil {
		appLogger.Warn("Failed to initialize stats disk store, running with in-memory stats only", slog.Any("error", err))
	}

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

	// 4. Attach RingBuffer and EventBroker sinks for Management API (v0.6.0+)
	ringBuffer := telemetry.NewRingBufferSink(500)
	tracker.AddSink(ringBuffer)

	eventBroker := telemetry.NewEventBroker()
	tracker.AddSink(eventBroker)

	// 5. Attach Streaming TrafficLogger sink for Auto-Tuner
	var trafficLogger *telemetry.TrafficLogger
	trafficLogger, err = telemetry.NewTrafficLogger("", 5000)
	if err == nil {
		tracker.AddSink(trafficLogger)
		if histRecords, rErr := telemetry.ReadRecords(trafficLogger.FilePath(), 50); rErr == nil && len(histRecords) > 0 {
			for _, rec := range histRecords {
				ringBuffer.Emit(rec)
			}
			appLogger.Info("Hydrated recent routes ring buffer from disk", slog.Int("count", len(histRecords)))
		}
	} else {
		appLogger.Warn("Failed to initialize traffic logger", slog.Any("error", err))
	}

	// Periodic stats sync to disk (every 1 minute)
	if diskStore != nil {
		go func() {
			interval := p.statsSyncInterval
			if interval == 0 {
				interval = 1 * time.Minute
			}
			ticker := time.NewTicker(interval)
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
	srvHandler.SetRingBuffer(ringBuffer)
	srvHandler.SetEventBroker(eventBroker)
	activeConfigPath := contract.DefaultConfigFileName
	if *configPathFlag != "" {
		activeConfigPath = *configPathFlag
	}
	srvHandler.SetConfigPath(activeConfigPath)

	// Automatic zero-downtime background file watcher for config.yaml changes
	go func(cfgFile string) {
		var lastMod time.Time
		if stat, err := os.Stat(cfgFile); err == nil {
			lastMod = stat.ModTime()
		}
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-bgCtx.Done():
				return
			case <-ticker.C:
				stat, err := os.Stat(cfgFile)
				if err == nil {
					if !lastMod.IsZero() && stat.ModTime().After(lastMod) {
						lastMod = stat.ModTime()
						appLogger.Info("🌮 Detected disk modification to config.yaml, hot-reloading...", slog.String("path", cfgFile))
						if reloadErr := srvHandler.ReloadConfigFromDisk(); reloadErr != nil {
							appLogger.Error("Failed to hot-reload config from disk", slog.Any("error", reloadErr))
						} else {
							appLogger.Info("✅ Configuration successfully hot-reloaded from disk", slog.String("path", cfgFile))
						}
					} else if lastMod.IsZero() {
						lastMod = stat.ModTime()
					}
				}
			}
		}
	}(activeConfigPath)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           srvHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	p.mu.Lock()
	if bgCtx.Err() != nil {
		p.mu.Unlock()
		return nil
	}
	p.cancelBg = bgCancel
	p.store = diskStore
	p.tracker = tracker
	p.trafficLog = trafficLogger
	p.server = srv
	p.mu.Unlock()

	appLogger.Info("🌮 Nacho Flow starting",
		slog.String("address", fmt.Sprintf("http://%s", addr)),
		slog.Int("providers_count", len(reg.All())),
		slog.String("brand", "spicebox.dev"),
	)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		if isAddressInUse(err) {
			fmt.Fprintf(os.Stderr, "[FATAL:PORT_IN_USE:%d] Port %d is already in use by another application. Please free port %d or change the port in config.yaml.\n", cfg.Port, cfg.Port, cfg.Port)
			appLogger.Error("Port bind collision", slog.Int("port", cfg.Port), slog.String("code", "PORT_IN_USE"), slog.Any("error", err))
		} else {
			fmt.Fprintf(os.Stderr, "[FATAL:SERVER_ERROR] %v\n", err)
			appLogger.Error("HTTP server error", slog.Any("error", err))
		}
		_ = p.Stop(s)
		return err
	}
	return nil
}

func (p *program) Stop(s service.Service) error {
	p.mu.Lock()
	cancelBg := p.cancelBg
	srv := p.server
	tracker := p.tracker
	diskStore := p.store
	trafficLog := p.trafficLog
	logCloser := p.logCloser
	p.mu.Unlock()

	if cancelBg != nil {
		cancelBg()
	}
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	if tracker != nil {
		tracker.Flush()
		if diskStore != nil {
			_ = diskStore.Save(tracker.GetStats())
		}
		tracker.Close()
	}
	if trafficLog != nil {
		_ = trafficLog.Close()
	}
	if logCloser != nil {
		_ = logCloser.Close()
	}
	return nil
}

func handleTuneSubcommand(args []string) {
	if err := runTune(args); err != nil {
		logFatal(err)
	}
}

func runTune(args []string) error {
	tuneFlags := flag.NewFlagSet("tune", flag.ContinueOnError)
	configPath := tuneFlags.String("config", "config.yaml", "Path to config.yaml file")
	trafficLogPath := tuneFlags.String("traffic-log", "logs/traffic.jsonl", "Path to traffic log JSONL")
	sampleLimit := tuneFlags.Int("sample", 5000, "Maximum historical prompt turns to analyze")
	apply := tuneFlags.Bool("apply", false, "Apply recommended rule optimizations to config.yaml")

	if err := tuneFlags.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", *configPath, err)
	}

	records, err := telemetry.ReadRecords(*trafficLogPath, *sampleLimit)
	if err != nil {
		return fmt.Errorf("failed to read traffic log at %s: %w", *trafficLogPath, err)
	}

	if len(records) == 0 {
		fmt.Printf("ℹ️  No historical traffic records found in %s.\n", *trafficLogPath)
		fmt.Printf("   Run Nacho Flow under normal traffic to accumulate telemetry before tuning.\n")
		return nil
	}

	optimizer := tuner.NewCostPenaltyOptimizer()
	result, err := optimizer.Optimize(records, cfg)
	if err != nil {
		return fmt.Errorf("optimization analysis failed: %w", err)
	}

	// Generate and print human-readable report
	report := tuner.GenerateAdvisoryReport(result, cfg)
	fmt.Print(report)

	if *apply {
		backupPath, err := applyTuningFunc(*configPath, result)
		if err != nil {
			return fmt.Errorf("failed to apply tuning: %w", err)
		}
		fmt.Printf("✅ SUCCESS: Successfully updated %s with optimal rules!\n", *configPath)
		fmt.Printf("   Backup saved at: %s\n", backupPath)
		fmt.Printf("   Restart or reload nacho-flow to activate changes.\n\n")
	}
	return nil
}

func handleServiceControl(s service.Service, args []string) (bool, error) {
	if len(args) > 1 {
		cmd := args[1]
		if cmd == "service" || cmd == "svc" {
			if len(args) > 2 {
				subCmd := args[2]
				if err := service.Control(s, subCmd); err != nil {
					return true, fmt.Errorf("service control error (%s): %w", subCmd, err)
				}
				fmt.Printf("[nacho-flow] Service %s executed successfully.\n", subCmd)
				return true, nil
			}
		}
	}
	return false, nil
}

var (
	serviceControlRunner = handleServiceControl
	defaultServiceRunner = func(s service.Service) error {
		setupShutdownSignal(s)
		return s.Run()
	}
)

func runMain(args []string, serviceRunner func(service.Service) error) error {
	if len(args) > 1 {
		cmd := args[1]
		switch cmd {
		case "version", "-v", "--version":
			fmt.Printf("nacho-flow %s (spicebox.dev/nacho-flow)\n", contract.Version)
			return nil
		case "tune":
			return runTune(args[2:])
		case "deals", "deal", "hunt", "heat-seek", "heatseek":
			return runDeals(args[2:])
		}
	}

	if *versionFlag || *vFlag {
		fmt.Printf("nacho-flow %s (spicebox.dev/nacho-flow)\n", contract.Version)
		return nil
	}

	svcConfig := &service.Config{
		Name:        "nacho-flow",
		DisplayName: "Nacho Flow Agent Supervisor & Model Dispatcher",
		Description: "Agent Supervisor & Model Dispatcher for coding agents (spicebox.dev/nacho-flow)",
	}

	prg := &program{}
	s, err := serviceNewFunc(prg, svcConfig)
	if err != nil {
		return fmt.Errorf("service initialization error: %w", err)
	}

	// Handle service commands (install, uninstall, start, stop)
	if handled, err := serviceControlRunner(s, args); handled {
		return err
	}

	if serviceRunner != nil {
		return serviceRunner(s)
	}

	return defaultServiceRunner(s)
}

func setupShutdownSignal(s service.Service) chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		if _, ok := <-sigChan; ok {
			log.Println("[nacho-flow] Received shutdown signal, stopping...")
			_ = s.Stop()
		}
	}()
	return sigChan
}

func runDeals(args []string) error {
	fs := flag.NewFlagSet("deals", flag.ContinueOnError)
	port := fs.Int("port", contract.DefaultServerPort, "Nacho Flow daemon port")
	host := fs.String("host", contract.DefaultDaemonHost, "Nacho Flow daemon host")
	asJSON := fs.Bool("json", false, "Output deals as raw JSON")
	apiKey := fs.String("auth", "", "Daemon auth token (if required)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	daemonURL := fmt.Sprintf("%s://%s:%d", contract.HTTPProtocol, *host, *port)
	return runDealsCommand(daemonURL, *apiKey, *asJSON, os.Stdout)
}

func fetchDeals(daemonURL, apiKey string) (*server.DealsResponse, error) {
	reqURL := fmt.Sprintf("%s%s", strings.TrimRight(daemonURL, "/"), contract.PathAPIDeals)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create deals request: %w", err)
	}

	if apiKey != "" {
		req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nacho-flow daemon at %s: %w", daemonURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var dealsResp server.DealsResponse
	if err := json.NewDecoder(resp.Body).Decode(&dealsResp); err != nil {
		return nil, fmt.Errorf("failed to decode deals response: %w", err)
	}
	return &dealsResp, nil
}

func runDealsCommand(daemonURL, apiKey string, asJSON bool, out io.Writer) error {
	dealsResp, err := fetchDeals(daemonURL, apiKey)
	if err != nil {
		return err
	}

	reporter := NewDealsReporter(asJSON)
	return reporter.Render(out, *dealsResp)
}

func main() {
	flag.Parse()
	if err := runMain(os.Args, nil); err != nil {
		logFatal(err)
	}
}
