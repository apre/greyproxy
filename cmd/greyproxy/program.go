package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/greyhavenhq/greyproxy/internal/gostcore/auth"
	"github.com/greyhavenhq/greyproxy/internal/gostcore/logger"
	"github.com/greyhavenhq/greyproxy/internal/gostcore/service"
	greyproxy "github.com/greyhavenhq/greyproxy/internal/greyproxy"
	greyproxy_api "github.com/greyhavenhq/greyproxy/internal/greyproxy/api"
	greyproxy_plugins "github.com/greyhavenhq/greyproxy/internal/greyproxy/plugins"
	greyproxy_ui "github.com/greyhavenhq/greyproxy/internal/greyproxy/ui"
	api_service "github.com/greyhavenhq/greyproxy/internal/gostx/api/service"
	xauth "github.com/greyhavenhq/greyproxy/internal/gostx/auth"
	"github.com/greyhavenhq/greyproxy/internal/gostx/config"
	"github.com/greyhavenhq/greyproxy/internal/gostx/config/loader"
	auth_parser "github.com/greyhavenhq/greyproxy/internal/gostx/config/parsing/auth"
	"github.com/greyhavenhq/greyproxy/internal/gostx/config/parsing/parser"
	xmetrics "github.com/greyhavenhq/greyproxy/internal/gostx/metrics"
	metrics "github.com/greyhavenhq/greyproxy/internal/gostx/metrics/service"
	"github.com/greyhavenhq/greyproxy/internal/gostx/registry"
	"github.com/judwhite/go-svc"
	"github.com/spf13/viper"
)

type program struct {
	srvApi       service.Service
	srvMetrics   service.Service
	srvGreyproxy *greyproxy.Service
	srvProfiling *http.Server

	cancel context.CancelFunc
}

func (p *program) Init(env svc.Environment) error {
	parser.Init(parser.Args{
		CfgFile:     cfgFile,
		Services:    services,
		Nodes:       nodes,
		Debug:       debug,
		Trace:       trace,
		ApiAddr:     apiAddr,
		MetricsAddr: metricsAddr,
	})

	return nil
}

func (p *program) Start() error {
	cfg, err := parser.Parse()
	if err != nil {
		return err
	}

	if outputFormat != "" {
		if err := cfg.Write(os.Stdout, outputFormat); err != nil {
			return err
		}
		os.Exit(0)
	}

	config.Set(cfg)

	// Override DNS handler to capture responses for DNS cache population.
	// Must happen before loader.Load creates services.
	greyproxy_plugins.OverrideDNSHandler()

	if err := loader.Load(cfg); err != nil {
		return err
	}

	if err := p.run(cfg); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.reload(ctx)

	return nil
}

func (p *program) run(cfg *config.Config) error {
	for _, svc := range registry.ServiceRegistry().GetAll() {
		svc := svc
		go func() {
			svc.Serve()
		}()
	}

	if p.srvApi != nil {
		p.srvApi.Close()
		p.srvApi = nil
	}
	if cfg.API != nil {
		s, err := buildApiService(cfg.API)
		if err != nil {
			return err
		}

		p.srvApi = s

		go func() {
			defer s.Close()

			log := logger.Default().WithFields(map[string]any{"kind": "service", "service": "@api"})

			log.Info("listening on ", s.Addr())
			if err := s.Serve(); !errors.Is(err, http.ErrServerClosed) {
				log.Error(err)
			}
		}()
	}

	if p.srvMetrics != nil {
		p.srvMetrics.Close()
		p.srvMetrics = nil
	}
	if cfg.Metrics != nil && cfg.Metrics.Addr != "" {
		s, err := buildMetricsService(cfg.Metrics)
		if err != nil {
			return err
		}

		p.srvMetrics = s

		xmetrics.Enable(true)

		go func() {
			defer s.Close()

			log := logger.Default().WithFields(map[string]any{"kind": "service", "service": "@metrics"})

			log.Info("listening on ", s.Addr())
			if err := s.Serve(); !errors.Is(err, http.ErrServerClosed) {
				log.Error(err)
			}
		}()
	}

	if p.srvProfiling != nil {
		p.srvProfiling.Close()
		p.srvProfiling = nil
	}
	if cfg.Profiling != nil {
		addr := cfg.Profiling.Addr
		if addr == "" {
			addr = ":6060"
		}
		s := &http.Server{
			Addr: addr,
		}
		p.srvProfiling = s

		go func() {
			defer s.Close()

			log := logger.Default().WithFields(map[string]any{"kind": "service", "service": "@profiling"})

			log.Info("listening on ", addr)
			if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				log.Error(err)
			}
		}()
	}

	// Build and start greyproxy service if configured
	if p.srvGreyproxy == nil {
		if err := p.buildGreyproxyService(); err != nil {
			logger.Default().Warnf("greyproxy: %v", err)
		}
	}

	return nil
}

func (p *program) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}

	for name, srv := range registry.ServiceRegistry().GetAll() {
		srv.Close()
		logger.Default().Debugf("service %s shutdown", name)
	}

	if p.srvApi != nil {
		p.srvApi.Close()
		logger.Default().Debug("service @api shutdown")
	}
	if p.srvMetrics != nil {
		p.srvMetrics.Close()
		logger.Default().Debug("service @metrics shutdown")
	}
	if p.srvProfiling != nil {
		p.srvProfiling.Close()
		logger.Default().Debug("service @profiling shutdown")
	}
	if p.srvGreyproxy != nil {
		p.srvGreyproxy.Close()
		logger.Default().Debug("service @greyproxy shutdown")
	}

	return nil
}

func (p *program) reload(ctx context.Context) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	for {
		select {
		case <-c:
			if err := p.reloadConfig(); err != nil {
				logger.Default().Error(err)
			} else {
				logger.Default().Info("config reloaded")
			}

		case <-ctx.Done():
			return
		}
	}
}

func (p *program) reloadConfig() error {
	cfg, err := parser.Parse()
	if err != nil {
		return err
	}
	config.Set(cfg)

	if err := loader.Load(cfg); err != nil {
		return err
	}

	if err := p.run(cfg); err != nil {
		return err
	}

	return nil
}

func buildApiService(cfg *config.APIConfig) (service.Service, error) {
	var authers []auth.Authenticator
	if auther := auth_parser.ParseAutherFromAuth(cfg.Auth); auther != nil {
		authers = append(authers, auther)
	}
	if cfg.Auther != "" {
		authers = append(authers, registry.AutherRegistry().Get(cfg.Auther))
	}

	var auther auth.Authenticator
	if len(authers) > 0 {
		auther = xauth.AuthenticatorGroup(authers...)
	}

	network := "tcp"
	addr := cfg.Addr
	if strings.HasPrefix(addr, "unix://") {
		network = "unix"
		addr = strings.TrimPrefix(addr, "unix://")
	}
	return api_service.NewService(
		network, addr,
		api_service.PathPrefixOption(cfg.PathPrefix),
		api_service.AccessLogOption(cfg.AccessLog),
		api_service.AutherOption(auther),
	)
}

func (p *program) buildGreyproxyService() error {
	// Read greyproxy config from the same config file using viper
	var gaCfg greyproxy.Config
	if err := viper.UnmarshalKey("greyproxy", &gaCfg); err != nil {
		return nil // No greyproxy section, skip silently
	}
	if gaCfg.Addr == "" {
		return nil // Not configured
	}

	if gaCfg.PathPrefix == "" {
		gaCfg.PathPrefix = "/"
	}
	if gaCfg.DB == "" {
		gaCfg.DB = filepath.Join(greyproxyDataHome(), "greyproxy.db")
	}
	if gaCfg.Auther == "" {
		gaCfg.Auther = "auther-0"
	}
	if gaCfg.Admission == "" {
		gaCfg.Admission = "admission-0"
	}
	if gaCfg.Bypass == "" {
		gaCfg.Bypass = "bypass-0"
	}
	if gaCfg.Resolver == "" {
		gaCfg.Resolver = "resolver-0"
	}

	log := logger.Default().WithFields(map[string]any{"kind": "service", "service": "@greyproxy"})

	// Create shared state (this also opens the DB)
	shared := &greyproxy_api.Shared{}

	// Create a temporary service to get DB, cache, bus
	tmpSvc, err := greyproxy.NewService(&gaCfg, nil)
	if err != nil {
		return err
	}

	shared.DB = tmpSvc.DB
	shared.Cache = tmpSvc.Cache
	shared.Bus = tmpSvc.Bus

	// Set the shared DNS cache so the DNS handler wrapper can populate it
	greyproxy_plugins.SetSharedDNSCache(shared.Cache)

	// Create and register gost plugins
	autherPlugin := greyproxy_plugins.NewAuther()
	admissionPlugin := greyproxy_plugins.NewAdmission()
	bypassPlugin := greyproxy_plugins.NewBypass(shared.DB, shared.Cache, shared.Bus)
	resolverPlugin := greyproxy_plugins.NewResolver(shared.Cache)

	registry.AutherRegistry().Register(gaCfg.Auther, autherPlugin)
	registry.AdmissionRegistry().Register(gaCfg.Admission, admissionPlugin)
	registry.BypassRegistry().Register(gaCfg.Bypass, bypassPlugin)
	registry.ResolverRegistry().Register(gaCfg.Resolver, resolverPlugin)

	log.Infof("plugins registered: auther=%s admission=%s bypass=%s resolver=%s",
		gaCfg.Auther, gaCfg.Admission, gaCfg.Bypass, gaCfg.Resolver)

	// Build HTTP router with REST API + HTMX UI + WebSocket
	router, g := greyproxy_api.NewRouter(shared, gaCfg.PathPrefix)
	greyproxy_ui.RegisterPageRoutes(g, shared.DB, shared.Bus)
	greyproxy_ui.RegisterHTMXRoutes(g, shared.DB, shared.Bus)

	// Create the actual service
	svc := &greyproxy.Service{}
	*svc = *tmpSvc
	svc.SetHandler(router)

	p.srvGreyproxy = svc

	go func() {
		log.Info("listening on ", svc.Addr())
		if err := svc.Serve(); !errors.Is(err, http.ErrServerClosed) {
			log.Error(err)
		}
	}()

	return nil
}

func buildMetricsService(cfg *config.MetricsConfig) (service.Service, error) {
	auther := auth_parser.ParseAutherFromAuth(cfg.Auth)
	if cfg.Auther != "" {
		auther = registry.AutherRegistry().Get(cfg.Auther)
	}

	network := "tcp"
	addr := cfg.Addr
	if strings.HasPrefix(addr, "unix://") {
		network = "unix"
		addr = strings.TrimPrefix(addr, "unix://")
	}
	return metrics.NewService(
		network, addr,
		metrics.PathOption(cfg.Path),
		metrics.AutherOption(auther),
	)
}

// greyproxyDataHome returns the directory for Greyproxy data files.
// Priority: GREYPROXY_DATA_HOME > XDG_DATA_HOME/greyproxy > current directory.
func greyproxyDataHome() string {
	if v := os.Getenv("GREYPROXY_DATA_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "greyproxy")
	}
	return "."
}
